package rest

import (
	"encoding/json"
	"net/http"

	"github.com/syntrixbase/syntrix/pkg/model"
)

func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	var q model.Query
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validateQuery(q); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid query parameters")
		return
	}

	database, ok := h.databaseOrError(w, r)
	if !ok {
		return
	}

	docs, err := h.engine.ExecuteQuery(r.Context(), database, q)
	if err != nil {
		writeInternalError(w, err, "Failed to execute query")
		return
	}

	writeJSON(w, http.StatusOK, docs)
}
