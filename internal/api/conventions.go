package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// defaultPageSize is the page size when a list request names none. The
// description declares the same default.
const defaultPageSize = 50

// etagOf renders a version as a strong ETag.
func etagOf(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

// versionOf reads the version an If-Match header names. A header that names
// no version of ours matches nothing, so the change fails with 412.
func versionOf(ifMatch string) (int64, error) {
	tag := strings.TrimPrefix(strings.TrimSpace(ifMatch), "W/")
	version, err := strconv.ParseInt(strings.Trim(tag, `"`), 10, 64)
	if err != nil || version <= 0 {
		return 0, newProblem(http.StatusPreconditionFailed, "the If-Match header names no version of this resource")
	}
	return version, nil
}

// encodeCursor and decodeCursor keep cursors opaque to clients (ADR-043). A
// cursor names the last item of a page, and the next page continues after
// it in the list's order.
func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func decodeCursor(cursor v1gen.OptString) (int64, error) {
	encoded, ok := cursor.Get()
	if !ok || encoded == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, invalidCursor()
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidCursor()
	}
	return id, nil
}

func invalidCursor() error {
	return fieldProblem(http.StatusBadRequest, "the cursor is not one this API issued",
		"cursor", "is not a cursor this API issued")
}

// pageOf trims items, which the store read with one item beyond limit, to
// one page. It returns the cursor of the next page, which is unset on the
// last page.
func pageOf[T any](items []T, limit int, idOf func(T) int64) ([]T, v1gen.OptString) {
	if len(items) <= limit {
		return items, v1gen.OptString{}
	}
	page := items[:limit]
	return page, v1gen.NewOptString(encodeCursor(idOf(page[limit-1])))
}

// storeProblem maps the store's errors to problems and passes any other
// error on.
func storeProblem(err error) error {
	var field *store.FieldError
	if errors.As(err, &field) {
		return storeFieldProblem(field)
	}
	switch {
	case errors.Is(err, store.ErrInUse):
		return newProblem(http.StatusConflict, "the item is in use: remove what uses it first")
	case errors.Is(err, store.ErrNotFound):
		return newProblem(http.StatusNotFound, "")
	case errors.Is(err, store.ErrVersionMismatch):
		return newProblem(http.StatusPreconditionFailed, "the resource changed since it was read: read it again and repeat the change")
	case errors.Is(err, store.ErrLastAdministrator):
		return newProblem(http.StatusConflict, "the instance needs at least one Instance Admin")
	case errors.Is(err, store.ErrAdministratorExists):
		return newProblem(http.StatusConflict, "the setup is complete: an Instance Admin exists")
	case errors.Is(err, store.ErrNameTaken):
		return fieldProblem(http.StatusConflict, "another item has this name", "name", "is taken")
	case errors.Is(err, store.ErrSetupLinkSpent):
		return newProblem(http.StatusForbidden, "the setup link is used, expired or unknown: run the bootstrap command for a new one")
	}
	return err
}

// storeFieldProblem maps a store error about one field of a change to a
// problem that names the field.
func storeFieldProblem(field *store.FieldError) error {
	switch {
	case errors.Is(field.Err, store.ErrNameTaken):
		return fieldProblem(http.StatusConflict, "another item has this name", field.Field, "is taken")
	case errors.Is(field.Err, store.ErrDuplicate):
		return fieldProblem(http.StatusConflict, "another item is the same", field.Field, "repeats another item")
	case errors.Is(field.Err, store.ErrInUse):
		return fieldProblem(http.StatusConflict, "a route still uses what the change would remove", field.Field, "is used by a route")
	case errors.Is(field.Err, store.ErrUnknownReference):
		return fieldProblem(http.StatusUnprocessableEntity, "an id names no item", field.Field, "names no item")
	case errors.Is(field.Err, store.ErrNotTheTeams):
		return fieldProblem(http.StatusForbidden, "a route connects only the team's own sources and sinks", field.Field, "is not the team's")
	}
	return field
}
