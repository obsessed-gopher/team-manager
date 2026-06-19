// Package httpx содержит вспомогательные функции для работы с HTTP JSON API.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Error — доменная ошибка с HTTP-статусом, понятная транспортному слою.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// NewError создаёт доменную HTTP-ошибку.
func NewError(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

// Часто используемые ошибки-конструкторы.
func BadRequest(msg string) *Error   { return NewError(http.StatusBadRequest, msg) }
func Unauthorized(msg string) *Error { return NewError(http.StatusUnauthorized, msg) }
func Forbidden(msg string) *Error    { return NewError(http.StatusForbidden, msg) }
func NotFound(msg string) *Error     { return NewError(http.StatusNotFound, msg) }
func Conflict(msg string) *Error     { return NewError(http.StatusConflict, msg) }

// JSON пишет данные в ответ в формате JSON.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if payload == nil {
		return
	}

	_ = json.NewEncoder(w).Encode(payload)
}

// Fail отображает ошибку в JSON-ответ. Доменные *Error дают свой статус,
// прочие ошибки — 500.
func Fail(w http.ResponseWriter, err error) {
	var de *Error
	if errors.As(err, &de) {
		JSON(w, de.Status, map[string]string{"error": de.Message})
		return
	}

	JSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

// Decode читает и валидирует JSON-тело запроса.
func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return BadRequest("invalid request body: " + err.Error())
	}

	return nil
}
