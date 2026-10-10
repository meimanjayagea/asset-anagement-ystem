package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) assetPhotos(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	return s.rows(w, r, `SELECT p.id,p.purpose,p.caption,p.width,p.height,p.created_at,p.uploaded_by FROM asset_photos p JOIN assets a ON a.org_id=p.org_id AND a.id=p.asset_id JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE p.org_id=$1 AND p.asset_id=$2 AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id) ORDER BY p.id`, actor(r).OrgID, n, actor(r).ID)
}

func (s *Server) uploadPhoto(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Purpose string `json:"purpose"`
		Caption string `json:"caption"`
		Data    string `json:"data"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	capability := map[string]string{"catalog": "assets.write", "damage": "maintenance.request", "repair": "maintenance.manage", "disposal": "requests.create", "checkout": "loans.manage", "return": "loans.manage", "audit": "stocktakes.observe"}[in.Purpose]
	if capability == "" || !hasCapability(actor(r).Role, capability) {
		return fail(403, "Jenis foto tidak diizinkan untuk peran ini")
	}
	if len(in.Caption) > 200 || len(in.Data) > 700000 {
		return fail(422, "Foto maksimal 512 KB dan caption maksimal 200 karakter")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(in.Data)
	if err != nil || len(data) > 524288 {
		return fail(422, "Data foto tidak valid")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 {
		return fail(422, "Gunakan JPEG/PNG maksimal 2048 x 2048")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fail(422, "Foto rusak")
	}
	var normalized bytes.Buffer
	if err = jpeg.Encode(&normalized, decoded, &jpeg.Options{Quality: 85}); err != nil || normalized.Len() > 524288 {
		return fail(422, "Foto terlalu besar; kompres sebelum unggah")
	}
	var photoID int64
	err = s.transaction(r, func(tx pgx.Tx) error {
		a, err := lockAsset(r.Context(), tx, actor(r).OrgID, n)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(7842303)`); err != nil {
			return err
		}
		var count int
		var used int64
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE asset_id=$2),COALESCE(sum(octet_length(content)),0) FROM asset_photos WHERE org_id=$1`, actor(r).OrgID, n).Scan(&count, &used); err != nil {
			return err
		}
		if count >= 40 || used+int64(normalized.Len()) > 50*1024*1024 {
			return fail(409, "Batas foto tercapai: 40 per aset / 50 MB per organisasi")
		}
		if err = tx.QueryRow(r.Context(), `INSERT INTO asset_photos(org_id,asset_id,purpose,caption,content,width,height,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, actor(r).OrgID, n, in.Purpose, strings.TrimSpace(in.Caption), normalized.Bytes(), config.Width, config.Height, actor(r).ID).Scan(&photoID); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, actor(r), "photo_upload", "asset", a.ID, nil, map[string]any{"photo_id": photoID, "purpose": in.Purpose, "caption": in.Caption})
	})
	if err != nil {
		return err
	}
	write(w, 201, map[string]int64{"id": photoID})
	return nil
}

func (s *Server) photoContent(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var data []byte
	err = s.DB.QueryRow(r.Context(), `SELECT p.content FROM asset_photos p JOIN assets a ON a.org_id=p.org_id AND a.id=p.asset_id JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE p.org_id=$1 AND p.id=$2 AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id)`, actor(r).OrgID, n, actor(r).ID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(404, "Foto tidak ditemukan")
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(200)
	_, err = w.Write(data)
	return err
}

func attachEvidence(r *http.Request, tx pgx.Tx, asset int64, photos []int64, purpose, entity string, entityID int64, required bool) error {
	if len(photos) > 10 || (required && len(photos) == 0) {
		return fail(422, "Lampirkan 1-10 foto bukti kondisi")
	}
	seen := map[int64]bool{}
	for _, photo := range photos {
		if photo < 1 || seen[photo] {
			return fail(422, "Foto bukti tidak valid atau duplikat")
		}
		seen[photo] = true
		var valid bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM asset_photos WHERE org_id=$1 AND id=$2 AND asset_id=$3 AND purpose=$4)`, actor(r).OrgID, photo, asset, purpose).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return fail(422, "Foto harus berasal dari aset dan jenis bukti yang sesuai")
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO asset_photo_links(org_id,photo_id,entity,entity_id) VALUES($1,$2,$3,$4)`, actor(r).OrgID, photo, entity, entityID); err != nil {
			return err
		}
	}
	return nil
}
