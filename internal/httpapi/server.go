package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"templates-registry/internal/domain"
)

type Server struct {
	users     domain.UserRepository
	versions  domain.DBVersionRepository
	entities  domain.EntityRepository
	revisions domain.TemplateRevisionRepository
	tx        domain.TransactionManager
	jwtSecret []byte
	tokenTTL  time.Duration
	logger    *log.Logger
}

type Config struct {
	JWTSecret string
	TokenTTL  time.Duration
	Logger    *log.Logger
}

// responseRecorder сохраняет статус ответа для итогового лога запроса.
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader перехватывает код ответа перед отправкой клиенту.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write перехватывает размер тела ответа для итогового логирования запроса.
func (r *responseRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytesWritten += n
	return n, err
}

// NewServer собирает HTTP API и подключает middleware логирования.
func NewServer(
	users domain.UserRepository,
	versions domain.DBVersionRepository,
	entities domain.EntityRepository,
	revisions domain.TemplateRevisionRepository,
	tx domain.TransactionManager,
	cfg Config,
) http.Handler {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = 24 * time.Hour
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	s := &Server{
		users:     users,
		versions:  versions,
		entities:  entities,
		revisions: revisions,
		tx:        tx,
		jwtSecret: []byte(cfg.JWTSecret),
		tokenTTL:  cfg.TokenTTL,
		logger:    cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("POST /token", s.handleToken)
	mux.HandleFunc("GET /db_version", s.handleDBVersion)
	mux.HandleFunc("GET /entities", s.handleListEntities)
	mux.HandleFunc("POST /entities/templates/batch", s.handleBatchTemplates)
	mux.HandleFunc("GET /entities/{entity_id}/templates", s.handleListTemplates)
	mux.HandleFunc("GET /entities/{entity_id}/revisions", s.handleListRevisionMeta)
	mux.HandleFunc("GET /entities/{entity_id}/revisions/{revision}", s.handleGetRevision)
	mux.HandleFunc("POST /entities", s.requireActiveUser(s.handleCreateEntity))
	mux.HandleFunc("PUT /entities/{entity_id}", s.requireActiveUser(s.handleUpdateEntity))
	mux.HandleFunc("DELETE /entities/{entity_id}", s.requireActiveUser(s.handleDeleteEntity))
	mux.HandleFunc("GET /users", s.requireModerator(s.handleListUsers))
	mux.HandleFunc("POST /users/{username}/ban", s.requireModerator(s.handleBanUser))
	mux.HandleFunc("POST /users/{username}/unban", s.requireModerator(s.handleUnbanUser))
	mux.HandleFunc("PUT /users/{username}/role", s.requireAdmin(s.handleSetRole))

	return s.withRequestLogging(mux)
}

// userClaims хранит полезную нагрузку JWT-токена пользователя.
type userClaims struct {
	jwt.RegisteredClaims
}

// userCreateRequest описывает запрос на регистрацию пользователя.
type userCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// roleUpdateRequest описывает запрос на смену роли пользователя.
type roleUpdateRequest struct {
	Role string `json:"role"`
}

// banRequest описывает запрос на временную или постоянную блокировку.
type banRequest struct {
	Days      *int `json:"days"`
	Permanent bool `json:"permanent"`
}

// batchTemplateRequest описывает пакетный запрос шаблонов.
type batchTemplateRequest struct {
	EntityIDs []string `json:"entity_ids"`
}

// entityCreateRequest описывает запрос на создание сущности.
type entityCreateRequest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TemplateID    string `json:"templateId"`
	Type          string `json:"type"`
	Template      string `json:"template"`
	Visualization string `json:"visualization"`
}

// entityUpdateRequest описывает запрос на обновление сущности.
type entityUpdateRequest struct {
	Name          string `json:"name"`
	TemplateID    string `json:"templateId"`
	Type          string `json:"type"`
	Template      string `json:"template"`
	Visualization string `json:"visualization"`
}

// userResponse описывает пользователя в административной выдаче.
type userResponse struct {
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	IsPermanentBan bool       `json:"is_permanent_ban"`
	BannedUntil    *time.Time `json:"banned_until"`
}

// templateResponse описывает шаблон в контракте клиента.
type templateResponse struct {
	EntityID      string `json:"entity_id"`
	Revision      int    `json:"revision"`
	TemplateID    string `json:"templateId"`
	Template      string `json:"template"`
	Author        string `json:"author"`
	Visualization string `json:"visualization"`
}

// handleHealth отвечает на проверку доступности сервера.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.logInfo("Выполнена проверка состояния сервера")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRegister регистрирует нового пользователя.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req userCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.logError("Не удалось разобрать запрос на регистрацию: %v", err)
		writeDetail(w, http.StatusBadRequest, "Некорректное тело запроса")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		s.logInfo("Отклонена регистрация без обязательных полей")
		writeDetail(w, http.StatusBadRequest, "Необходимо указать имя пользователя и пароль")
		return
	}
	s.logInfo("Попытка регистрации пользователя: username=%s", req.Username)

	if _, err := s.users.GetByUsername(r.Context(), req.Username); err == nil {
		s.logInfo("Регистрация отклонена: пользователь уже существует: username=%s", req.Username)
		writeDetail(w, http.StatusBadRequest, "Пользователь уже существует")
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		s.logError("Ошибка проверки существования пользователя %s: %v", req.Username, err)
		writeDetail(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logError("Ошибка хэширования пароля пользователя %s: %v", req.Username, err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось обработать пароль")
		return
	}

	user := &domain.User{
		ID:           uuid.New(),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         domain.RoleUser,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.users.Create(r.Context(), user); err != nil {
		s.logError("Ошибка сохранения пользователя %s: %v", req.Username, err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось создать пользователя")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Успешно"})
}

// handleToken выполняет вход пользователя и выдает JWT-токен.
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	username, password, err := readCredentials(r)
	if err != nil {
		s.logError("Не удалось прочитать учетные данные для входа: %v", err)
		writeDetail(w, http.StatusBadRequest, "Некорректные учетные данные")
		return
	}
	s.logInfo("Попытка входа пользователя: username=%s", username)

	user, err := s.users.GetByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logInfo("Вход отклонен: пользователь не найден: username=%s", username)
			writeDetail(w, http.StatusUnauthorized, "Неверные учетные данные")
			return
		}
		s.logError("Ошибка чтения пользователя %s при входе: %v", username, err)
		writeDetail(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		s.logInfo("Вход отклонен: неверный пароль для пользователя %s", username)
		writeDetail(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}

	token, err := s.issueToken(user.Username)
	if err != nil {
		s.logError("Ошибка генерации токена для пользователя %s: %v", user.Username, err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось выдать токен")
		return
	}

	s.logInfo("Пользователь успешно вошел в систему: username=%s role=%s", user.Username, user.Role)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": token,
		"token_type":   "bearer",
		"role":         user.Role,
	})
}

// handleDBVersion возвращает версию базы данных и глобальную ревизию.
func (s *Server) handleDBVersion(w http.ResponseWriter, r *http.Request) {
	s.logInfo("Запрошена версия базы данных: пользователь=%s", s.actorFromRequest(r))
	version, err := s.versions.Get(r.Context())
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		s.logError("Ошибка чтения версии базы данных: %v", err)
		writeDetail(w, status, "Не удалось получить версию базы данных")
		return
	}

	writeJSON(w, http.StatusOK, version)
}

// handleListEntities возвращает список сущностей с учетом фильтров.
func (s *Server) handleListEntities(w http.ResponseWriter, r *http.Request) {
	filter := domain.EntityListFilter{
		Type:           strings.TrimSpace(r.URL.Query().Get("type")),
		IncludeDeleted: parseBool(r.URL.Query().Get("include_deleted"), true),
	}
	if activeOnly := r.URL.Query().Get("active_only"); activeOnly != "" {
		filter.IncludeDeleted = !parseBool(activeOnly, false)
	}
	s.logInfo("Запрошен список сущностей: пользователь=%s тип=%s include_deleted=%t", s.actorFromRequest(r), filter.Type, filter.IncludeDeleted)

	entities, err := s.entities.List(r.Context(), filter)
	if err != nil {
		s.logError("Ошибка чтения списка сущностей: %v", err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось получить список сущностей")
		return
	}

	s.logInfo("Список сущностей успешно прочитан: количество=%d", len(entities))
	writeJSON(w, http.StatusOK, entities)
}

// handleBatchTemplates возвращает последние шаблоны по списку сущностей.
func (s *Server) handleBatchTemplates(w http.ResponseWriter, r *http.Request) {
	var req batchTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}

	items, err := s.revisions.GetLatestByEntityIDs(r.Context(), req.EntityIDs)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]templateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toTemplateResponse(item))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleListTemplates возвращает историю ревизий шаблона по сущности.
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entity_id")
	s.logInfo("Запрошена история шаблонов: пользователь=%s entity_id=%s", s.actorFromRequest(r), entityID)
	items, err := s.revisions.ListByEntity(r.Context(), entityID)
	if err != nil {
		s.logError("Ошибка чтения истории шаблонов для сущности %s: %v", entityID, err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось получить историю шаблонов")
		return
	}

	resp := make([]templateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toTemplateResponse(item))
	}

	s.logInfo("История шаблонов успешно прочитана: entity_id=%s количество=%d", entityID, len(resp))
	writeJSON(w, http.StatusOK, resp)
}

// handleListRevisionMeta возвращает метаданные ревизий для клиента.
func (s *Server) handleListRevisionMeta(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entity_id")
	items, err := s.revisions.ListRevisionMetaByEntity(r.Context(), entityID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// handleGetRevision возвращает конкретную ревизию шаблона.
func (s *Server) handleGetRevision(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entity_id")
	revision, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid revision")
		return
	}

	item, err := s.revisions.GetByEntityRevision(r.Context(), entityID, revision)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toTemplateResponse(*item))
}

// handleCreateEntity создает сущность и первую ревизию шаблона.
func (s *Server) handleCreateEntity(w http.ResponseWriter, r *http.Request, user *domain.User) {
	var req entityCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.logError("Не удалось разобрать запрос на создание сущности от пользователя %s: %v", user.Username, err)
		writeDetail(w, http.StatusBadRequest, "Некорректное тело запроса")
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.TemplateID) == "" || strings.TrimSpace(req.Type) == "" {
		s.logInfo("Отклонено создание сущности без обязательных полей: пользователь=%s", user.Username)
		writeDetail(w, http.StatusBadRequest, "Необходимо указать id, name, templateId и type")
		return
	}
	s.logInfo("Попытка создания сущности: пользователь=%s entity_id=%s template_id=%s", user.Username, req.ID, req.TemplateID)

	if err := s.tx.Do(r.Context(), func(ctx context.Context) error {
		now := time.Now().UTC()
		entity := &domain.Entity{
			ID:        req.ID,
			Name:      req.Name,
			Type:      req.Type,
			Deleted:   false,
			Revision:  1,
			CreatedBy: &user.ID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.entities.Create(ctx, entity); err != nil {
			return err
		}

		revision := &domain.TemplateRevision{
			ID:            req.TemplateID,
			EntityID:      req.ID,
			Revision:      1,
			Template:      req.Template,
			Visualization: req.Visualization,
			Author:        user.Username,
			CreatedBy:     &user.ID,
			CreatedAt:     now,
		}
		if err := s.revisions.Create(ctx, revision); err != nil {
			return err
		}

		return s.versions.Increment(ctx)
	}); err != nil {
		s.logError("Ошибка создания сущности %s пользователем %s: %v", req.ID, user.Username, err)
		writeDetail(w, http.StatusInternalServerError, "Не удалось создать сущность")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Успешно"})
}

// handleUpdateEntity создает новую ревизию существующей сущности.
func (s *Server) handleUpdateEntity(w http.ResponseWriter, r *http.Request, user *domain.User) {
	entityID := r.PathValue("entity_id")
	var req entityUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.TemplateID) == "" {
		writeDetail(w, http.StatusBadRequest, "name and templateId are required")
		return
	}

	entity, err := s.entities.GetByID(r.Context(), entityID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}
	if entity.Deleted {
		writeDetail(w, http.StatusNotFound, "entity is deleted")
		return
	}

	entityType := entity.Type
	if strings.TrimSpace(req.Type) != "" {
		entityType = req.Type
	}

	if err := s.tx.Do(r.Context(), func(ctx context.Context) error {
		revisionNumber, err := s.entities.Update(ctx, entityID, req.Name, entityType)
		if err != nil {
			return err
		}

		revision := &domain.TemplateRevision{
			ID:            req.TemplateID,
			EntityID:      entityID,
			Revision:      revisionNumber,
			Template:      req.Template,
			Visualization: req.Visualization,
			Author:        user.Username,
			CreatedBy:     &user.ID,
			CreatedAt:     time.Now().UTC(),
		}
		if err := s.revisions.Create(ctx, revision); err != nil {
			return err
		}

		return s.versions.Increment(ctx)
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Успешно"})
}

// handleDeleteEntity выполняет мягкое удаление сущности.
func (s *Server) handleDeleteEntity(w http.ResponseWriter, r *http.Request, _ *domain.User) {
	entityID := r.PathValue("entity_id")

	if err := s.tx.Do(r.Context(), func(ctx context.Context) error {
		if _, err := s.entities.SoftDelete(ctx, entityID); err != nil {
			return err
		}
		return s.versions.Increment(ctx)
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Успешно"})
}

// handleListUsers возвращает список пользователей для панели управления.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, _ *domain.User) {
	users, err := s.users.List(r.Context())
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]userResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, userResponse{
			Username:       user.Username,
			Role:           user.Role,
			IsPermanentBan: user.IsPermanentBan,
			BannedUntil:    user.BannedUntil,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleBanUser блокирует пользователя на срок или навсегда.
func (s *Server) handleBanUser(w http.ResponseWriter, r *http.Request, moderator *domain.User) {
	username := r.PathValue("username")
	var req banRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}

	target, err := s.users.GetByUsername(r.Context(), username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}
	if target.Role == domain.RoleAdmin {
		writeDetail(w, http.StatusForbidden, "admin cannot be banned")
		return
	}
	if req.Permanent && moderator.Role != domain.RoleAdmin {
		writeDetail(w, http.StatusForbidden, "only admin can set permanent ban")
		return
	}
	if !req.Permanent && (req.Days == nil || *req.Days <= 0) {
		writeDetail(w, http.StatusBadRequest, "days must be greater than zero")
		return
	}

	var bannedUntil *time.Time
	duration := req.Days
	if !req.Permanent {
		until := time.Now().UTC().AddDate(0, 0, *req.Days)
		bannedUntil = &until
	}

	if err := s.users.UpdateBan(r.Context(), username, req.Permanent, bannedUntil, duration); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Забанен"})
}

// handleUnbanUser снимает блокировку с пользователя.
func (s *Server) handleUnbanUser(w http.ResponseWriter, r *http.Request, _ *domain.User) {
	username := r.PathValue("username")
	if err := s.users.ClearBan(r.Context(), username); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Разбанен"})
}

// handleSetRole меняет роль пользователя.
func (s *Server) handleSetRole(w http.ResponseWriter, r *http.Request, _ *domain.User) {
	username := r.PathValue("username")
	var req roleUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role != domain.RoleUser && req.Role != domain.RoleModerator && req.Role != domain.RoleAdmin {
		writeDetail(w, http.StatusBadRequest, "invalid role")
		return
	}

	if err := s.users.UpdateRole(r.Context(), username, req.Role); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeDetail(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Роль изменена"})
}

// authedHandler описывает обработчик с уже загруженным пользователем.
type authedHandler func(http.ResponseWriter, *http.Request, *domain.User)

// requireActiveUser проверяет JWT и состояние блокировки пользователя.
func (s *Server) requireActiveUser(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			writeDetail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if user.IsPermanentBan {
			writeDetail(w, http.StatusForbidden, map[string]any{"banned": true, "permanent": true})
			return
		}
		if user.BannedUntil != nil {
			now := time.Now().UTC()
			if user.BannedUntil.After(now) {
				writeDetail(w, http.StatusForbidden, map[string]any{
					"banned":    true,
					"permanent": false,
					"days":      user.BanDurationDays,
					"until":     user.BannedUntil.Format(time.RFC3339),
				})
				return
			}
			if err := s.users.ClearBan(r.Context(), user.Username); err != nil {
				writeDetail(w, http.StatusInternalServerError, err.Error())
				return
			}
			user.IsPermanentBan = false
			user.BannedUntil = nil
			user.BanDurationDays = nil
		}

		next(w, r, user)
	}
}

// requireModerator ограничивает доступ модераторам и администраторам.
func (s *Server) requireModerator(next authedHandler) http.HandlerFunc {
	return s.requireActiveUser(func(w http.ResponseWriter, r *http.Request, user *domain.User) {
		if user.Role != domain.RoleAdmin && user.Role != domain.RoleModerator {
			writeDetail(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r, user)
	})
}

// requireAdmin ограничивает доступ только администраторам.
func (s *Server) requireAdmin(next authedHandler) http.HandlerFunc {
	return s.requireActiveUser(func(w http.ResponseWriter, r *http.Request, user *domain.User) {
		if user.Role != domain.RoleAdmin {
			writeDetail(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r, user)
	})
}

// currentUser валидирует JWT и загружает пользователя из базы.
func (s *Server) currentUser(r *http.Request) (*domain.User, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return nil, errors.New("missing bearer token")
	}

	tokenString := strings.TrimSpace(header[len("Bearer "):])
	token, err := jwt.ParseWithClaims(tokenString, &userClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*userClaims)
	if !ok || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("invalid token")
	}

	return s.users.GetByUsername(r.Context(), claims.Subject)
}

// issueToken выпускает новый JWT-токен для пользователя.
func (s *Server) issueToken(username string) (string, error) {
	now := time.Now().UTC()
	claims := &userClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// toTemplateResponse преобразует доменную ревизию в HTTP-ответ.
func toTemplateResponse(item domain.TemplateRevision) templateResponse {
	return templateResponse{
		EntityID:      item.EntityID,
		Revision:      item.Revision,
		TemplateID:    item.ID,
		Template:      item.Template,
		Author:        item.Author,
		Visualization: item.Visualization,
	}
}

// readCredentials извлекает логин и пароль из form-data или JSON.
func readCredentials(r *http.Request) (string, string, error) {
	if err := r.ParseForm(); err == nil {
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		if username != "" || password != "" {
			if username == "" || password == "" {
				return "", "", errors.New("username and password are required")
			}
			return username, password, nil
		}
	}

	var req userCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(req.Username) == "" || req.Password == "" {
		return "", "", errors.New("username and password are required")
	}

	return strings.TrimSpace(req.Username), req.Password, nil
}

// decodeJSON декодирует JSON-тело запроса в целевую структуру.
func decodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
}

// writeJSON отправляет JSON-ответ с указанным HTTP-статусом.
func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeDetail отправляет ошибку в едином формате API.
func writeDetail(w http.ResponseWriter, status int, detail interface{}) {
	writeJSON(w, status, map[string]interface{}{"detail": detail})
}

// parseBool безопасно разбирает булев параметр запроса.
func parseBool(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// withRequestLogging пишет в журнал начало и завершение каждого HTTP-запроса.
// withRequestLogging пишет журнал начала и завершения каждого запроса.
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		r, meta := s.withRequestMetadata(r)
		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		w.Header().Set(headerRequestID, meta.RequestID)

		logRequestStarted(meta, r.RemoteAddr)
		next.ServeHTTP(recorder, r)
		logRequestCompleted(meta, recorder.statusCode, recorder.bytesWritten, time.Since(startedAt))
	})
}

// actorFromRequest извлекает пользователя из токена для логирования публичных запросов.
// actorFromRequest определяет пользователя для логирования публичных запросов.
func (s *Server) actorFromRequest(r *http.Request) string {
	tokenString := bearerToken(r)
	if tokenString == "" {
		return "гость"
	}

	claims, err := s.parseClaims(tokenString)
	if err != nil || strings.TrimSpace(claims.Subject) == "" {
		return "неизвестный-пользователь"
	}

	return claims.Subject
}

// parseClaims валидирует JWT и возвращает его полезную нагрузку.
// parseClaims валидирует JWT и возвращает claims токена.
func (s *Server) parseClaims(tokenString string) (*userClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &userClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*userClaims)
	if !ok || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// bearerToken извлекает Bearer-токен из заголовка Authorization.
// bearerToken извлекает Bearer-токен из заголовка Authorization.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}

	return strings.TrimSpace(header[len("Bearer "):])
}

// logInfo пишет информационное сообщение в журнал сервера.
// logInfo пишет информационное сообщение в журнал сервера.
func (s *Server) logInfo(format string, args ...interface{}) {
	s.logger.Printf("ИНФО: "+format, args...)
}

// logError пишет сообщение об ошибке в журнал сервера.
// logError пишет сообщение об ошибке в журнал сервера.
func (s *Server) logError(format string, args ...interface{}) {
	s.logger.Printf("ОШИБКА: "+format, args...)
}
