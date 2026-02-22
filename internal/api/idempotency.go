package api

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flowforge/internal/database"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const idempotencyHeader = "Idempotency-Key"
const idempotencyMaxBodyBytes int64 = 32 * 1024
const idempotencyMaxKeyLen = 128

type idempotencyContext struct {
	Key         string
	Endpoint    string
	RequestHash string
}

func beginIdempotentMutation(w http.ResponseWriter, r *http.Request, endpoint string) (*idempotencyContext, bool) {
	key, err := parseIdempotencyKey(r)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
		return nil, true
	}
	if key == "" {
		return nil, false
	}
	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("idempotency database init failed: %v", err))
			return nil, true
		}
	}

	requestHash, err := buildRequestHash(r)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, fmt.Sprintf("invalid request body for idempotency: %v", err))
		return nil, true
	}

	record, err := database.GetControlPlaneReplay(key, endpoint)
	if err == nil {
		reqID := requestIDFromRequest(r)
		if strings.TrimSpace(record.RequestHash) != requestHash {
			apiMetrics.IncControlPlaneIdempotencyConflict()
			_ = database.LogAuditEventWithRequestID("control-plane", "IDEMPOTENT_CONFLICT", annotateReasonWithRequestID("idempotency key reused with different request payload", r), "api", 0, endpoint, reqID)
			writeJSONErrorForRequest(w, r, http.StatusConflict, "idempotency key reused with different request payload")
			return nil, true
		}

		_ = database.TouchControlPlaneReplay(record.ID)
		apiMetrics.IncControlPlaneIdempotentReplay()
		_ = database.LogAuditEventWithRequestID("control-plane", "IDEMPOTENT_REPLAY", annotateReasonWithRequestID("served cached control-plane mutation response", r), "api", 0, endpoint, reqID)
		w.Header().Set("X-Idempotent-Replay", "true")
		writeRawJSON(w, record.ResponseStatus, record.ResponseBody)
		return nil, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("idempotency lookup failed: %v", err))
		return nil, true
	}

	return &idempotencyContext{
		Key:         key,
		Endpoint:    endpoint,
		RequestHash: requestHash,
	}, false
}

func persistIdempotentMutation(ctx *idempotencyContext, statusCode int, payload interface{}) {
	if ctx == nil {
		return
	}
	if statusCode >= 500 {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = database.InsertControlPlaneReplay(ctx.Key, ctx.Endpoint, ctx.RequestHash, statusCode, string(raw))
}

func parseIdempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get(idempotencyHeader))
	if key == "" {
		return "", nil
	}
	if len(key) > idempotencyMaxKeyLen {
		return "", fmt.Errorf("idempotency key exceeds %d characters", idempotencyMaxKeyLen)
	}
	for _, ch := range key {
		if ch < 33 || ch > 126 {
			return "", errors.New("idempotency key must contain only visible ASCII characters")
		}
	}
	return key, nil
}

func buildRequestHash(r *http.Request) (string, error) {
	bodyBytes, err := readAndRestoreRequestBody(r, idempotencyMaxBodyBytes)
	if err != nil {
		return "", err
	}
	canonical := canonicalizeBody(bodyBytes)
	sum := sha256.Sum256([]byte(r.Method + "\n" + r.URL.Path + "\n" + canonical))
	return hex.EncodeToString(sum[:]), nil
}

func canonicalizeBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "{}"
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return trimmed
	}
	return string(canonical)
}

func readAndRestoreRequestBody(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bodyBytes)) > limit {
		return nil, fmt.Errorf("request body exceeds %d bytes", limit)
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func writeRawJSON(w http.ResponseWriter, statusCode int, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	if statusCode >= 400 {
		w.Header().Set("Content-Type", "application/problem+json")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(raw))
	_, _ = w.Write([]byte("\n"))
}
