package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var accountCodePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)
var financeZone = time.FixedZone("WIB", 7*60*60)

type valuationBasis struct {
	Amount int64
	Life   int
	Start  time.Time
}

type financeQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assetBookValueAt(ctx context.Context, q financeQuerier, orgID int64, a Asset, asOf time.Time) (book, gross, accumulated int64, err error) {
	start, parseErr := time.Parse("2006-01-02", a.DepStart)
	if parseErr != nil {
		return 0, 0, 0, parseErr
	}
	basis := valuationBasis{Amount: a.Cost, Life: a.Life, Start: start}
	var valuationDate time.Time
	err = q.QueryRow(ctx, `SELECT revalued_amount,remaining_life_months,effective_date FROM asset_valuations WHERE org_id=$1 AND asset_id=$2 AND status='approved' AND effective_date<=$3::date ORDER BY effective_date DESC,id DESC LIMIT 1`, orgID, a.ID, asOf.Format("2006-01-02")).Scan(&basis.Amount, &basis.Life, &valuationDate)
	if err == nil {
		basis.Start = valuationDate
	} else if err != pgx.ErrNoRows {
		return 0, 0, 0, err
	}
	gross = basis.Amount
	salvage := a.Salvage
	if salvage > gross {
		salvage = gross
	}
	book = depreciationBookValue(a.Depreciation, gross, salvage, basis.Life, basis.Start, asOf)
	accumulated = gross - book
	return book, gross, accumulated, nil
}

func assetPeriodCharge(ctx context.Context, q financeQuerier, orgID int64, a Asset, start, end time.Time) (int64, error) {
	basisStart, err := time.Parse("2006-01-02", a.DepStart)
	if err != nil {
		return 0, err
	}
	basis := valuationBasis{Amount: a.Cost, Life: a.Life, Start: basisStart}
	var effective time.Time
	err = q.QueryRow(ctx, `SELECT revalued_amount,remaining_life_months,effective_date FROM asset_valuations WHERE org_id=$1 AND asset_id=$2 AND status='approved' AND effective_date<=$3::date ORDER BY effective_date DESC,id DESC LIMIT 1`, orgID, a.ID, start.Format("2006-01-02")).Scan(&basis.Amount, &basis.Life, &effective)
	if err == nil {
		basis.Start = effective
	} else if err != pgx.ErrNoRows {
		return 0, err
	}
	salvage := a.Salvage
	if salvage > basis.Amount {
		salvage = basis.Amount
	}
	opening := depreciationBookValue(a.Depreciation, basis.Amount, salvage, basis.Life, basis.Start, start.AddDate(0, 0, -1))
	closing := depreciationBookValue(a.Depreciation, basis.Amount, salvage, basis.Life, basis.Start, end)
	charge := opening - closing
	if charge < 0 {
		charge = 0
	}
	return charge, nil
}

func assetRemainingLifeAt(ctx context.Context, q financeQuerier, orgID int64, a Asset, asOf time.Time) (int, error) {
	start, err := time.Parse("2006-01-02", a.DepStart)
	if err != nil {
		return 0, err
	}
	life := a.Life
	var valuationDate time.Time
	err = q.QueryRow(ctx, `SELECT remaining_life_months,effective_date FROM asset_valuations WHERE org_id=$1 AND asset_id=$2 AND status='approved' AND effective_date<=$3::date ORDER BY effective_date DESC,id DESC LIMIT 1`, orgID, a.ID, asOf.Format("2006-01-02")).Scan(&life, &valuationDate)
	if err == nil {
		start = valuationDate
	} else if err != pgx.ErrNoRows {
		return 0, err
	}
	remaining := life - elapsedMonths(start, asOf)
	if remaining < 1 {
		remaining = 1
	}
	return remaining, nil
}

func parseFinancePeriod(raw string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", raw, financeZone)
	if err != nil || start.Format("2006-01") != raw {
		return time.Time{}, time.Time{}, fail(422, "Periode harus berformat YYYY-MM")
	}
	return start, start.AddDate(0, 1, 0).Add(-time.Nanosecond), nil
}

func (s *Server) depreciationReport(w http.ResponseWriter, r *http.Request) error {
	start, end, err := parseFinancePeriod(r.URL.Query().Get("period"))
	if err != nil {
		return err
	}
	u := actor(r)
	branch := selectedBranch(r)
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.tag,a.name,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.depreciation_method,a.depreciation_start_date::text,l.branch_id,b.code AS branch_code FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id JOIN branches b ON b.org_id=l.org_id AND b.id=l.branch_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND a.status<>'disposed' AND l.deleted_at IS NULL AND b.deleted_at IS NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) ORDER BY b.code,a.tag`, u.OrgID, u.ID, branch)
	if err != nil {
		return err
	}
	defer rows.Close()
	type line struct {
		AssetID     int64  `json:"asset_id"`
		Tag         string `json:"tag"`
		Name        string `json:"name"`
		Branch      string `json:"branch_code"`
		Method      string `json:"depreciation_method"`
		Opening     int64  `json:"opening_book_value"`
		Charge      int64  `json:"depreciation_charge"`
		Revaluation int64  `json:"revaluation_change"`
		Closing     int64  `json:"closing_book_value"`
		BranchID    int64  `json:"branch_id"`
	}
	out := []line{}
	for rows.Next() {
		var a Asset
		var purchase, depStart string
		var branchID int64
		var branchCode string
		if err = rows.Scan(&a.ID, &a.Tag, &a.Name, &purchase, &a.Cost, &a.Salvage, &a.Life, &a.Depreciation, &depStart, &branchID, &branchCode); err != nil {
			return err
		}
		a.PurchaseDate, a.DepStart, a.BranchID = purchase, depStart, branchID
		openingDate := start.AddDate(0, 0, -1)
		opening, _, _, e := assetBookValueAt(r.Context(), s.DB, u.OrgID, a, openingDate)
		if e != nil {
			return e
		}
		closingDate := time.Date(end.Year(), end.Month(), end.Day(), 12, 0, 0, 0, financeZone)
		closing, _, _, e := assetBookValueAt(r.Context(), s.DB, u.OrgID, a, closingDate)
		if e != nil {
			return e
		}
		charge, e := assetPeriodCharge(r.Context(), s.DB, u.OrgID, a, start, closingDate)
		if e != nil {
			return e
		}
		out = append(out, line{AssetID: a.ID, Tag: a.Tag, Name: a.Name, Branch: branchCode, Method: a.Depreciation, Opening: opening, Charge: charge, Revaluation: closing - opening + charge, Closing: closing, BranchID: branchID})
	}
	if err = rows.Err(); err != nil {
		return err
	}
	write(w, 200, map[string]any{"period": start.Format("2006-01"), "items": out})
	return nil
}

func (s *Server) complianceReport(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	branch := selectedBranch(r)
	var total, missingTag, expiredWarranty, overdueMaintenance, archived int64
	var expiredContracts int64
	businessDate := time.Now().In(financeZone).Format("2006-01-02")
	err := s.DB.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE a.deleted_at IS NULL AND a.status<>'disposed'),count(*) FILTER (WHERE a.deleted_at IS NULL AND btrim(a.tag)=''),count(*) FILTER (WHERE a.deleted_at IS NULL AND a.status<>'disposed' AND a.warranty_until<$4::date),count(*) FILTER (WHERE a.deleted_at IS NULL AND a.status<>'disposed' AND a.next_maintenance_date<$4::date),count(*) FILTER (WHERE a.deleted_at IS NOT NULL) FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3)`, u.OrgID, u.ID, branch, businessDate).Scan(&total, &missingTag, &expiredWarranty, &overdueMaintenance, &archived)
	if err != nil {
		return err
	}
	err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM service_contracts c WHERE c.org_id=$1 AND c.deleted_at IS NULL AND c.end_date<$4::date AND can_access_branch(c.org_id,$2,c.branch_id) AND ($3::bigint=0 OR c.branch_id=$3)`, u.OrgID, u.ID, branch, businessDate).Scan(&expiredContracts)
	if err != nil {
		return err
	}
	write(w, 200, map[string]any{"assets_active": total, "assets_archived": archived, "assets_missing_tag": missingTag, "warranties_expired": expiredWarranty, "maintenance_overdue": overdueMaintenance, "contracts_expired": expiredContracts, "as_of": time.Now().In(financeZone).Format("2006-01-02")})
	return nil
}

func (s *Server) listContracts(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	financial := hasCapability(u.Role, "assets.finance")
	p, size := page(r)
	return s.rows(w, r, `SELECT c.id,c.branch_id,b.code AS branch_code,c.asset_id,a.tag,c.name,c.vendor,c.contract_number,c.start_date::text,c.end_date::text,c.renewal_notice_days,CASE WHEN $6 THEN c.annual_cost ELSE NULL END AS annual_cost,c.notes,(c.end_date-$7::date)<=c.renewal_notice_days AS renewal_due,c.deleted_at FROM service_contracts c JOIN branches b ON b.org_id=c.org_id AND b.id=c.branch_id LEFT JOIN assets a ON a.org_id=c.org_id AND a.id=c.asset_id WHERE c.org_id=$1 AND c.deleted_at IS NULL AND b.deleted_at IS NULL AND can_access_branch(c.org_id,$2,c.branch_id) AND ($3::bigint=0 OR c.branch_id=$3) ORDER BY c.end_date,c.id LIMIT $4 OFFSET $5`, u.OrgID, u.ID, selectedBranch(r), size, (p-1)*size, financial, time.Now().In(financeZone).Format("2006-01-02"))
}

func (s *Server) createContract(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Branch     int64  `json:"branch_id"`
		Asset      *int64 `json:"asset_id"`
		Name       string `json:"name"`
		Vendor     string `json:"vendor"`
		Number     string `json:"contract_number"`
		Start      string `json:"start_date"`
		End        string `json:"end_date"`
		Notice     int    `json:"renewal_notice_days"`
		AnnualCost int64  `json:"annual_cost"`
		Notes      string `json:"notes"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	in.Name, in.Vendor = strings.TrimSpace(in.Name), strings.TrimSpace(in.Vendor)
	in.Number, in.Notes = strings.TrimSpace(in.Number), strings.TrimSpace(in.Notes)
	start, e1 := time.Parse("2006-01-02", in.Start)
	end, e2 := time.Parse("2006-01-02", in.End)
	if e1 != nil || e2 != nil || end.Before(start) || len(in.Name) < 2 || len(in.Name) > 200 || len(in.Vendor) < 2 || len(in.Vendor) > 200 || len(in.Number) > 100 || len(in.Notes) > 2000 || in.Notice < 0 || in.Notice > 3650 || in.AnnualCost < 0 || in.AnnualCost > 1_000_000_000_000_000 {
		return fail(422, "Data kontrak tidak valid")
	}
	var contractID int64
	err := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if in.Branch == 0 {
			in.Branch = selectedBranch(r)
		}
		if e := branchScope(r, tx, in.Branch); e != nil {
			return e
		}
		if in.Asset != nil {
			var assetBranch int64
			if e := tx.QueryRow(r.Context(), `SELECT l.branch_id FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id)`, u.OrgID, *in.Asset, u.ID).Scan(&assetBranch); e != nil {
				if e == pgx.ErrNoRows {
					return fail(404, "Aset kontrak tidak ditemukan")
				}
				return e
			}
			if assetBranch != in.Branch {
				return fail(422, "Cabang kontrak harus sama dengan cabang aset")
			}
		}
		if !hasCapability(u.Role, "assets.finance") {
			in.AnnualCost = 0
		}
		err := tx.QueryRow(r.Context(), `INSERT INTO service_contracts(org_id,branch_id,asset_id,name,vendor,contract_number,start_date,end_date,renewal_notice_days,annual_cost,notes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7::date,$8::date,$9,$10,$11,$12) RETURNING id`, u.OrgID, in.Branch, in.Asset, in.Name, in.Vendor, in.Number, in.Start, in.End, in.Notice, in.AnnualCost, in.Notes, u.ID).Scan(&contractID)
		if err != nil {
			return err
		}
		return logAudit(r.Context(), tx, u, "create", "contract", contractID, nil, in)
	})
	if err != nil {
		return err
	}
	write(w, 201, map[string]int64{"id": contractID})
	return nil
}

func (s *Server) archiveContract(w http.ResponseWriter, r *http.Request) error {
	id, err := id(r)
	if err != nil {
		return err
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var branch int64
		if e := tx.QueryRow(r.Context(), `SELECT branch_id FROM service_contracts WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, id).Scan(&branch); e != nil {
			if e == pgx.ErrNoRows {
				return fail(404, "Kontrak tidak ditemukan")
			}
			return e
		}
		if e := branchScope(r, tx, branch); e != nil {
			return e
		}
		if _, e := tx.Exec(r.Context(), `UPDATE service_contracts SET deleted_at=now(),updated_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, id); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "archive", "contract", id, map[string]int64{"branch_id": branch}, map[string]bool{"archived": true})
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) accountingSettings(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	if r.Method == http.MethodGet {
		return s.rows(w, r, `SELECT asset_account,accumulated_depreciation_account,depreciation_expense_account,cash_account,disposal_gain_account,disposal_loss_account,revaluation_reserve_account,impairment_expense_account,updated_at FROM accounting_profiles WHERE org_id=$1`, u.OrgID)
	}
	var in map[string]string
	if err := decode(w, r, &in); err != nil {
		return err
	}
	keys := []string{"asset_account", "accumulated_depreciation_account", "depreciation_expense_account", "cash_account", "disposal_gain_account", "disposal_loss_account", "revaluation_reserve_account", "impairment_expense_account"}
	values := make([]any, len(keys)+2)
	values[0], values[len(values)-1] = u.OrgID, u.ID
	for i, key := range keys {
		value := strings.TrimSpace(in[key])
		if !accountCodePattern.MatchString(value) {
			return fail(422, "Kode akun tidak valid: "+key)
		}
		values[i+1] = value
	}
	return s.transaction(r, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `INSERT INTO accounting_profiles(org_id,asset_account,accumulated_depreciation_account,depreciation_expense_account,cash_account,disposal_gain_account,disposal_loss_account,revaluation_reserve_account,impairment_expense_account,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(org_id) DO UPDATE SET asset_account=excluded.asset_account,accumulated_depreciation_account=excluded.accumulated_depreciation_account,depreciation_expense_account=excluded.depreciation_expense_account,cash_account=excluded.cash_account,disposal_gain_account=excluded.disposal_gain_account,disposal_loss_account=excluded.disposal_loss_account,revaluation_reserve_account=excluded.revaluation_reserve_account,impairment_expense_account=excluded.impairment_expense_account,updated_by=excluded.updated_by,updated_at=now()`, values...)
		if err != nil {
			return err
		}
		return logAudit(r.Context(), tx, u, "update", "accounting_profile", u.OrgID, nil, in)
	})
}

func (s *Server) valuations(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	if r.Method == http.MethodGet {
		p, size := page(r)
		return s.rows(w, r, `SELECT v.id,v.asset_id,a.tag,a.name,v.branch_id,b.code AS branch_code,v.requested_by,requester.name AS requester,v.decided_by,decider.name AS decider,v.effective_date::text,CASE WHEN $6 THEN v.carrying_value_before ELSE NULL END AS carrying_value_before,CASE WHEN $6 THEN v.revalued_amount ELSE NULL END AS revalued_amount,v.remaining_life_months,v.reason,v.status,v.decision_note,v.expected_version,v.created_at,v.decided_at FROM asset_valuations v JOIN assets a ON a.org_id=v.org_id AND a.id=v.asset_id JOIN branches b ON b.org_id=v.org_id AND b.id=v.branch_id JOIN users requester ON requester.id=v.requested_by LEFT JOIN users decider ON decider.id=v.decided_by WHERE v.org_id=$1 AND can_access_branch(v.org_id,$2,v.branch_id) AND ($3::bigint=0 OR v.branch_id=$3) AND ($7 OR v.requested_by=$8) ORDER BY v.id DESC LIMIT $4 OFFSET $5`, u.OrgID, u.ID, selectedBranch(r), size, (p-1)*size, hasCapability(u.Role, "assets.finance"), hasCapability(u.Role, "finance.read"), u.ID)
	}
	var in struct {
		Asset  int64  `json:"asset_id"`
		Amount int64  `json:"revalued_amount"`
		Life   int    `json:"remaining_life_months"`
		Reason string `json:"reason"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Amount < 0 || in.Amount > 1_000_000_000_000_000 || in.Life < 0 || in.Life > 1200 || len(in.Reason) < 3 || len(in.Reason) > 1000 {
		return fail(422, "Data revaluasi tidak valid")
	}
	var valuationID int64
	err := s.transaction(r, func(tx pgx.Tx) error {
		a, err := lockAsset(r.Context(), tx, u.OrgID, in.Asset)
		if err != nil {
			return err
		}
		if a.Status == "disposed" {
			return fail(409, "Aset yang telah dilepas tidak dapat dinilai ulang")
		}
		if in.Amount < a.Salvage {
			return fail(422, "Nilai baru tidak boleh di bawah nilai residu")
		}
		if a.Version == 0 {
			return fail(409, "Versi aset tidak valid")
		}
		month := time.Now().In(financeZone)
		effective := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, financeZone)
		book, _, _, err := assetBookValueAt(r.Context(), tx, u.OrgID, a, effective.AddDate(0, 0, -1))
		if err != nil {
			return err
		}
		remaining, err := assetRemainingLifeAt(r.Context(), tx, u.OrgID, a, effective)
		if err != nil {
			return err
		}
		if in.Life == 0 || !hasCapability(u.Role, "assets.finance") {
			in.Life = remaining
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO asset_valuations(org_id,asset_id,branch_id,requested_by,effective_date,carrying_value_before,revalued_amount,remaining_life_months,reason,expected_version) VALUES($1,$2,$3,$4,$5::date,$6,$7,$8,$9,$10)`, u.OrgID, a.ID, a.BranchID, u.ID, effective.Format("2006-01-02"), book, in.Amount, in.Life, in.Reason, a.Version); err != nil {
			return err
		}
		err = tx.QueryRow(r.Context(), `SELECT id FROM asset_valuations WHERE org_id=$1 AND asset_id=$2 AND status='pending'`, u.OrgID, a.ID).Scan(&valuationID)
		if err != nil {
			return err
		}
		return logAudit(r.Context(), tx, u, "propose", "valuation", valuationID, map[string]int64{"carrying_value": book}, in)
	})
	if err != nil {
		return err
	}
	write(w, 201, map[string]int64{"id": valuationID})
	return nil
}

func mustDate(raw string) time.Time { t, _ := time.Parse("2006-01-02", raw); return t }

func (s *Server) decideValuation(w http.ResponseWriter, r *http.Request) error {
	id, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Note) > 1000 || (!in.Approve && len(in.Note) < 3) {
		return fail(422, "Alasan penolakan wajib; maksimal 1000 karakter")
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var assetID, proposer int64
		if e := tx.QueryRow(r.Context(), `SELECT asset_id,requested_by FROM asset_valuations WHERE org_id=$1 AND id=$2`, u.OrgID, id).Scan(&assetID, &proposer); e != nil {
			if e == pgx.ErrNoRows {
				return fail(404, "Revaluasi tidak ditemukan")
			}
			return e
		}
		a, e := lockAsset(r.Context(), tx, u.OrgID, assetID)
		if e != nil {
			return e
		}
		var status string
		var expected int
		if e = tx.QueryRow(r.Context(), `SELECT status,expected_version FROM asset_valuations WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, id).Scan(&status, &expected); e != nil {
			return e
		}
		if status != "pending" {
			return fail(409, "Revaluasi sudah diputuskan")
		}
		if proposer == u.ID {
			return fail(403, "Pengusul tidak dapat menyetujui revaluasinya sendiri")
		}
		if in.Approve && a.Version != expected {
			return fail(409, "Aset berubah; ajukan ulang revaluasi")
		}
		next := "rejected"
		if in.Approve {
			next = "approved"
		}
		if _, e = tx.Exec(r.Context(), `UPDATE asset_valuations SET status=$1,decided_by=$2,decision_note=$3,decided_at=now() WHERE org_id=$4 AND id=$5`, next, u.ID, in.Note, u.OrgID, id); e != nil {
			return e
		}
		if in.Approve {
			if _, e = tx.Exec(r.Context(), `UPDATE assets SET version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, assetID); e != nil {
				return e
			}
		}
		if e = logAudit(r.Context(), tx, u, next, "valuation", id, map[string]string{"status": status}, in); e != nil {
			return e
		}
		if in.Approve {
			return logAudit(r.Context(), tx, u, "revalue", "asset", assetID, a, map[string]any{"valuation_id": id, "version": a.Version + 1})
		}
		return nil
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

type journalLine struct {
	Date      string `json:"journal_date"`
	Branch    string `json:"branch_code"`
	Reference string `json:"reference"`
	Account   string `json:"account_code"`
	Debit     int64  `json:"debit"`
	Credit    int64  `json:"credit"`
	Memo      string `json:"memo"`
}

func (s *Server) journalLines(r *http.Request, start, end time.Time, branch int64) ([]journalLine, error) {
	u := actor(r)
	var accounts [8]string
	err := s.DB.QueryRow(r.Context(), `SELECT asset_account,accumulated_depreciation_account,depreciation_expense_account,cash_account,disposal_gain_account,disposal_loss_account,revaluation_reserve_account,impairment_expense_account FROM accounting_profiles WHERE org_id=$1`, u.OrgID).Scan(&accounts[0], &accounts[1], &accounts[2], &accounts[3], &accounts[4], &accounts[5], &accounts[6], &accounts[7])
	if err == pgx.ErrNoRows {
		accounts = [8]string{"1500", "1590", "6000", "1100", "7990", "6990", "3100", "6900"}
	} else if err != nil {
		return nil, err
	}
	out := []journalLine{}
	add := func(date, code, reference, account string, debit, credit int64, memo string) {
		if debit == 0 && credit == 0 {
			return
		}
		out = append(out, journalLine{Date: date, Branch: code, Reference: reference, Account: account, Debit: debit, Credit: credit, Memo: memo})
	}
	monthStart, monthEnd := start.Format("2006-01-02"), end.Format("2006-01-02")
	assetRows, err := s.DB.Query(r.Context(), `SELECT a.id,a.tag,a.name,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.depreciation_method,a.depreciation_start_date::text,b.code,l.branch_id FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id JOIN branches b ON b.org_id=l.org_id AND b.id=l.branch_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND a.status<>'disposed' AND l.deleted_at IS NULL AND b.deleted_at IS NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) ORDER BY a.id`, u.OrgID, u.ID, branch)
	if err != nil {
		return nil, err
	}
	for assetRows.Next() {
		var a Asset
		var code string
		if err = assetRows.Scan(&a.ID, &a.Tag, &a.Name, &a.PurchaseDate, &a.Cost, &a.Salvage, &a.Life, &a.Depreciation, &a.DepStart, &code, &a.BranchID); err != nil {
			assetRows.Close()
			return nil, err
		}
		charge, e := assetPeriodCharge(r.Context(), s.DB, u.OrgID, a, start, end)
		if e != nil {
			assetRows.Close()
			return nil, e
		}
		if charge > 0 {
			ref, memo, date := "DEP-"+a.Tag+"-"+start.Format("200601"), "Depreciation "+a.Tag+" / "+a.Name, monthEnd
			add(date, code, ref, accounts[2], charge, 0, memo)
			add(date, code, ref, accounts[1], 0, charge, memo)
		}
	}
	if err = assetRows.Err(); err != nil {
		assetRows.Close()
		return nil, err
	}
	assetRows.Close()
	disposals, err := s.DB.Query(r.Context(), `SELECT a.tag,a.name,b.code,(a.disposed_at AT TIME ZONE 'Asia/Jakarta')::date::text,a.disposal_proceeds,a.disposal_book_value,a.disposal_gross_value,a.disposal_accumulated_depreciation FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id JOIN branches b ON b.org_id=l.org_id AND b.id=l.branch_id WHERE a.org_id=$1 AND a.status='disposed' AND a.disposed_at IS NOT NULL AND a.disposal_book_value IS NOT NULL AND a.disposal_gross_value IS NOT NULL AND a.disposal_accumulated_depreciation IS NOT NULL AND (a.disposed_at AT TIME ZONE 'Asia/Jakarta')::date >= $2::date AND (a.disposed_at AT TIME ZONE 'Asia/Jakarta')::date < ($3::date+1) AND can_access_branch(a.org_id,$4,l.branch_id) AND ($5::bigint=0 OR l.branch_id=$5)`, u.OrgID, monthStart, monthEnd, u.ID, branch)
	if err != nil {
		return nil, err
	}
	for disposals.Next() {
		var tag, name, code, date string
		var proceeds, book, gross, accumulated int64
		if err = disposals.Scan(&tag, &name, &code, &date, &proceeds, &book, &gross, &accumulated); err != nil {
			disposals.Close()
			return nil, err
		}
		ref, memo := "DISP-"+tag+"-"+start.Format("200601"), "Asset disposal "+tag+" / "+name
		add(date, code, ref, accounts[3], proceeds, 0, memo)
		add(date, code, ref, accounts[1], accumulated, 0, memo)
		add(date, code, ref, accounts[0], 0, gross, memo)
		if proceeds > book {
			add(date, code, ref, accounts[4], 0, proceeds-book, memo)
		} else if book > proceeds {
			add(date, code, ref, accounts[5], book-proceeds, 0, memo)
		}
	}
	if err = disposals.Err(); err != nil {
		disposals.Close()
		return nil, err
	}
	disposals.Close()
	valuations, err := s.DB.Query(r.Context(), `SELECT a.tag,a.name,b.code,v.effective_date::text,v.carrying_value_before,v.revalued_amount FROM asset_valuations v JOIN assets a ON a.org_id=v.org_id AND a.id=v.asset_id JOIN branches b ON b.org_id=v.org_id AND b.id=v.branch_id WHERE v.org_id=$1 AND v.status='approved' AND v.effective_date >= $2::date AND v.effective_date <= $3::date AND can_access_branch(v.org_id,$4,v.branch_id) AND ($5::bigint=0 OR v.branch_id=$5)`, u.OrgID, monthStart, monthEnd, u.ID, branch)
	if err != nil {
		return nil, err
	}
	for valuations.Next() {
		var tag, name, code, date string
		var oldValue, newValue int64
		if err = valuations.Scan(&tag, &name, &code, &date, &oldValue, &newValue); err != nil {
			valuations.Close()
			return nil, err
		}
		ref, memo := "REVAL-"+tag+"-"+start.Format("200601"), "Revaluation "+tag+" / "+name
		if newValue > oldValue {
			delta := newValue - oldValue
			add(date, code, ref, accounts[0], delta, 0, memo)
			add(date, code, ref, accounts[6], 0, delta, memo)
		} else if oldValue > newValue {
			delta := oldValue - newValue
			add(date, code, ref, accounts[7], delta, 0, memo)
			add(date, code, ref, accounts[0], 0, delta, memo)
		}
	}
	if err = valuations.Err(); err != nil {
		valuations.Close()
		return nil, err
	}
	valuations.Close()
	return out, nil
}

func (s *Server) journalPreview(w http.ResponseWriter, r *http.Request) error {
	start, end, err := parseFinancePeriod(r.URL.Query().Get("period"))
	if err != nil {
		return err
	}
	lines, err := s.journalLines(r, start, end, selectedBranch(r))
	if err != nil {
		return err
	}
	write(w, 200, map[string]any{"period": start.Format("2006-01"), "lines": lines})
	return nil
}

func (s *Server) exportJournal(w http.ResponseWriter, r *http.Request) error {
	if !hasCapability(actor(r).Role, "finance.read") {
		return fail(403, "Akses jurnal finance ditolak")
	}
	var in struct {
		Period string `json:"period"`
		Branch int64  `json:"branch_id"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.Branch < 0 {
		return fail(422, "Cabang tidak valid")
	}
	if in.Branch == 0 {
		in.Branch = selectedBranch(r)
	}
	if in.Branch > 0 {
		var ok bool
		if err := s.DB.QueryRow(r.Context(), `SELECT can_access_branch($1,$2,$3)`, actor(r).OrgID, actor(r).ID, in.Branch).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return fail(403, "Cabang di luar akses")
		}
	}
	start, end, err := parseFinancePeriod(in.Period)
	if err != nil {
		return err
	}
	lines, err := s.journalLines(r, start, end, in.Branch)
	if err != nil {
		return err
	}
	out := [][]string{{"journal_date", "branch_code", "reference", "account_code", "debit", "credit", "memo"}}
	for _, line := range lines {
		out = append(out, []string{safeCSV(line.Date), safeCSV(line.Branch), safeCSV(line.Reference), safeCSV(line.Account), strconv.FormatInt(line.Debit, 10), strconv.FormatInt(line.Credit, 10), safeCSV(line.Memo)})
	}
	if err = s.transaction(r, func(tx pgx.Tx) error {
		return logAudit(r.Context(), tx, actor(r), "export_journal_csv", "accounting_export", 0, nil, map[string]any{"period": in.Period, "branch_id": in.Branch, "lines": len(lines)})
	}); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="asset-journal-%s.csv"`, in.Period))
	w.WriteHeader(200)
	_, _ = w.Write([]byte("\xef\xbb\xbf"))
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	writer.WriteAll(out)
	return writer.Error()
}
