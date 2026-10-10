package app

import (
	"errors"
	"time"
)

var transitions = map[string]map[string]bool{
	"available":   {"assign": true, "maintenance_start": true, "transfer": true, "dispose": true},
	"assigned":    {"return": true},
	"maintenance": {"maintenance_complete": true},
}

func allowed(status, action string) bool { return transitions[status][action] }

// Integer rupiah. Month-based straight-line estimate, never a tax posting.
func bookValue(cost, salvage int64, life int, purchase, asOf time.Time) int64 {
	if life <= 0 || asOf.Before(purchase) {
		return cost
	}
	months := (asOf.Year()-purchase.Year())*12 + int(asOf.Month()-purchase.Month())
	if asOf.Day() < purchase.Day() {
		months--
	}
	if months < 0 {
		months = 0
	}
	if months >= life {
		return salvage
	}
	return cost - (cost-salvage)/int64(life)*int64(months) - (cost-salvage)%int64(life)*int64(months)/int64(life)
}

func depreciationBookValue(method string, cost, salvage int64, life int, startDate, asOf time.Time) int64 {
	if method == "non_depreciable" || life <= 0 || asOf.Before(startDate) {
		return cost
	}
	months := elapsedMonths(startDate, asOf)
	if months <= 0 {
		return cost
	}
	if method != "declining_balance" {
		return bookValue(cost, salvage, life, startDate, asOf)
	}
	carrying := cost
	for month := 1; month <= months && month <= life; month++ {
		remaining := int64(life - month + 1)
		balance := carrying - salvage
		if balance <= 0 {
			return salvage
		}
		doubleDeclining := carrying * 2 / int64(life)
		straightLine := (balance + remaining - 1) / remaining
		charge := doubleDeclining
		if straightLine > charge {
			charge = straightLine
		}
		if charge > balance {
			charge = balance
		}
		carrying -= charge
	}
	if months >= life || carrying < salvage {
		return salvage
	}
	return carrying
}

func elapsedMonths(startDate, asOf time.Time) int {
	months := (asOf.Year()-startDate.Year())*12 + int(asOf.Month()-startDate.Month())
	if asOf.Day() < startDate.Day() {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

func validateMoney(cost, salvage int64, life int) error {
	if cost < 0 || cost > 1_000_000_000_000_000 || salvage < 0 || salvage > cost || life < 1 || life > 1200 {
		return errors.New("nilai atau masa manfaat tidak valid")
	}
	return nil
}
