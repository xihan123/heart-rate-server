package utils

import (
	"encoding/json"
	"heart-rate-server/internal/models"
	"net/http"
)

func SendResponse(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	w.WriteHeader(statusCode)
	err := json.NewEncoder(w).Encode(models.Response{
		Message: message,
		Data:    data,
	})
	if err != nil {
		return
	}
}

func SendError(w http.ResponseWriter, statusCode int, err error, message string) {
	w.WriteHeader(statusCode)
	resp := models.ErrorResponse{
		Message: message,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	err2 := json.NewEncoder(w).Encode(resp)
	if err2 != nil {
		return
	}
}
