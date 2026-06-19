// Package models содержит доменные модели сервиса управления задачами.
package models

import "time"

// Role — роль пользователя в команде.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Valid сообщает, является ли роль допустимой.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

// CanInvite сообщает, может ли роль приглашать участников.
func (r Role) CanInvite() bool {
	return r == RoleOwner || r == RoleAdmin
}

// TaskStatus — статус задачи.
type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

// Valid сообщает, является ли статус допустимым.
func (s TaskStatus) Valid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}

// User — пользователь сервиса.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Team — команда.
type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamMember — связь пользователя с командой и его роль в ней.
type TeamMember struct {
	TeamID   int64     `json:"team_id"`
	UserID   int64     `json:"user_id"`
	Role     Role      `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Task — задача.
type Task struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	AssigneeID  *int64     `json:"assignee_id,omitempty"`
	CreatedBy   int64      `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TaskHistory — запись аудита изменения задачи.
type TaskHistory struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	ChangedBy int64     `json:"changed_by"`
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	ChangedAt time.Time `json:"changed_at"`
}

// TaskComment — комментарий к задаче.
type TaskComment struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	UserID    int64     `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskFilter — параметры фильтрации и пагинации для списка задач.
type TaskFilter struct {
	TeamID     int64
	Status     TaskStatus
	AssigneeID *int64
	Limit      int
	Offset     int
}

// TeamStats — агрегированная статистика по команде (JOIN + агрегация).
type TeamStats struct {
	TeamID        int64  `json:"team_id"`
	TeamName      string `json:"team_name"`
	MembersCount  int    `json:"members_count"`
	DoneLast7Days int    `json:"done_tasks_last_7_days"`
}

// TopCreator — топ-пользователь по числу созданных задач в команде (оконная функция).
type TopCreator struct {
	TeamID       int64  `json:"team_id"`
	TeamName     string `json:"team_name"`
	UserID       int64  `json:"user_id"`
	UserName     string `json:"user_name"`
	TasksCreated int    `json:"tasks_created"`
	Rank         int    `json:"rank"`
}

// IntegrityIssue — задача с нарушением целостности (assignee не в команде).
type IntegrityIssue struct {
	TaskID     int64 `json:"task_id"`
	TeamID     int64 `json:"team_id"`
	AssigneeID int64 `json:"assignee_id"`
}
