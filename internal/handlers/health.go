package handlers

import (
	"heart-rate-server/internal/utils"
	"html/template"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/auth.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	err2 := tmpl.Execute(w, nil)
	if err2 != nil {
		return
	}
}

func (app *App) IndexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to load template")
		return
	}
	w.Header().Set("Content-Type", "text/html")
	err2 := tmpl.Execute(w, nil)
	if err2 != nil {
		return
	}
}
