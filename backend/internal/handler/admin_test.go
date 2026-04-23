package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// MergeConflict's input validation runs entirely before any DB call,
// so we can exercise it without a connection by routing through chi.
func TestMergeConflict_Validation(t *testing.T) {
	h := &AdminHandler{}
	r := chi.NewRouter()
	r.Post("/admin/conflicts/{id}/merge", h.MergeConflict)

	conflictID := uuid.New().String()

	cases := map[string]struct {
		body     string
		wantCode int
	}{
		"bad_json": {
			body:     "not json",
			wantCode: http.StatusBadRequest,
		},
		"bad_surviving_id": {
			body:     `{"surviving_id":"not-a-uuid","merged_id":"` + uuid.New().String() + `"}`,
			wantCode: http.StatusBadRequest,
		},
		"bad_merged_id": {
			body:     `{"surviving_id":"` + uuid.New().String() + `","merged_id":"oops"}`,
			wantCode: http.StatusBadRequest,
		},
		"identical_ids": {
			body: func() string {
				id := uuid.New().String()
				return `{"surviving_id":"` + id + `","merged_id":"` + id + `"}`
			}(),
			wantCode: http.StatusBadRequest,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost,
				"/admin/conflicts/"+conflictID+"/merge",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Errorf("status=%d want %d body=%s", rr.Code, tc.wantCode, rr.Body.String())
			}
		})
	}
}

func TestMergeConflict_BadConflictID(t *testing.T) {
	h := &AdminHandler{}
	r := chi.NewRouter()
	r.Post("/admin/conflicts/{id}/merge", h.MergeConflict)

	req := httptest.NewRequest(http.MethodPost, "/admin/conflicts/not-a-uuid/merge",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["code"] != "bad_request" {
		t.Errorf("code=%q", resp["code"])
	}
}
