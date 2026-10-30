package dynamic

import (
	"context"
	"fmt"

	"github.com/andrewbraga1/jaguarbase/internal/config"
)

// VaultProvider defines the contract for any PII tokenization backend (e.g., Redis).
type VaultProvider interface {
	Tokenize(ctx context.Context, columnName string, raw string) (string, error)
}

// ScannerProvider defines the contract for prompt injection detection (e.g., RE2 Scanner).
type ScannerProvider interface {
	CheckTaint(payload string) bool
}

// Processor applies manifestation rules dynamically to raw database rows.
type Processor struct {
	vault   VaultProvider
	scanner ScannerProvider
}

// NewProcessor initializes the dynamic rule engine.
func NewProcessor(v VaultProvider, s ScannerProvider) *Processor {
	return &Processor{
		vault:   v,
		scanner: s,
	}
}

// ProcessRow takes a raw row (map) and applies the schema's governance rules.
// It enforces Fail-Closed mechanics: if a rule is unknown or tokenization fails, it aborts.
func (p *Processor) ProcessRow(ctx context.Context, row map[string]any, schema []config.ColumnDef) (map[string]any, error) {
	// Pre-allocate map capacity to reduce GC pressure
	safeRow := make(map[string]any, len(row)+1)
	isTainted := false

	for _, colDef := range schema {
		val, exists := row[colDef.Column]
		if !exists || val == nil {
			safeRow[colDef.Column] = nil
			continue
		}

		switch colDef.Governance {
		case "pii_vault":
			strVal, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("Fail-Closed: pii_vault requires string type for column '%s'", colDef.Column)
			}

			token, err := p.vault.Tokenize(ctx, colDef.Column, strVal)
			if err != nil {
				return nil, fmt.Errorf("failed to tokenize column '%s': %w", colDef.Column, err)
			}
			safeRow[colDef.Column] = token

		case "untrusted_input":
			if strVal, ok := val.(string); ok && p.scanner.CheckTaint(strVal) {
				isTainted = true
			}
			safeRow[colDef.Column] = val // Keep original, but flag the entire row as tainted

		case "none", "":
			safeRow[colDef.Column] = val

		default:
			// Fail-closed on unrecognized governance directives
			return nil, fmt.Errorf("Fail-Closed: unknown governance rule '%s' for column '%s'", colDef.Governance, colDef.Column)
		}
	}

	// Append the system-level taint flag for the Execution Engine
	safeRow["_is_tainted"] = isTainted

	return safeRow, nil
}
