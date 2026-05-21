package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"payment-gateway/internal/dto"
)

const defaultReportLimit = 100
const maxReportLimit = 500

type storedReportTx struct {
	MerchantID     string
	PaymentID      string
	IdempotencyKey string
	Status         string
	PayloadJSON    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s *PostgresTransactionStore) BuildTransactionReport(ctx context.Context, filter dto.TransactionReportFilter) (dto.TransactionReport, error) {
	filter = normalizeTransactionReportFilter(filter)
	if filter.MerchantID == "" {
		return dto.TransactionReport{}, fmt.Errorf("merchant_id is required")
	}

	query := `
SELECT merchant_id, payment_id, idempotency_key, status, payload_json::text, created_at, updated_at
FROM payment_transactions
WHERE merchant_id = $1`
	args := []any{filter.MerchantID}
	next := 2

	if filter.DateFrom != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", next)
		args = append(args, *filter.DateFrom)
		next++
	}
	if filter.DateTo != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", next)
		args = append(args, *filter.DateTo)
		next++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND UPPER(status) = UPPER($%d)", next)
		args = append(args, filter.Status)
		next++
	}
	if filter.PaymentSystem != "" {
		query += fmt.Sprintf(" AND UPPER(COALESCE(payload_json->'transaction_details'->>'payment_system', '')) = UPPER($%d)", next)
		args = append(args, filter.PaymentSystem)
		next++
	}
	if filter.PaymentMethod != "" {
		query += fmt.Sprintf(" AND COALESCE(payload_json->'payment_info'->'payment_method_data'->>'type', '') = $%d", next)
		args = append(args, string(filter.PaymentMethod))
		next++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", next)
	args = append(args, filter.Limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return dto.TransactionReport{}, err
	}
	defer rows.Close()

	records := make([]storedReportTx, 0)
	for rows.Next() {
		var record storedReportTx
		if err := rows.Scan(
			&record.MerchantID,
			&record.PaymentID,
			&record.IdempotencyKey,
			&record.Status,
			&record.PayloadJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return dto.TransactionReport{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return dto.TransactionReport{}, err
	}

	return buildTransactionReport(filter, records), nil
}

func (s *InMemoryTransactionStore) BuildTransactionReport(ctx context.Context, filter dto.TransactionReportFilter) (dto.TransactionReport, error) {
	_ = ctx
	filter = normalizeTransactionReportFilter(filter)
	if filter.MerchantID == "" {
		return dto.TransactionReport{}, fmt.Errorf("merchant_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]storedReportTx, 0, len(s.byPaymentID))
	for key, tx := range s.byPaymentID {
		if !strings.HasPrefix(key, filter.MerchantID+":") {
			continue
		}

		record := storedReportTx{
			MerchantID:     filter.MerchantID,
			PaymentID:      strings.TrimPrefix(key, filter.MerchantID+":"),
			IdempotencyKey: tx.idempotencyKey,
			Status:         tx.status,
			PayloadJSON:    tx.payloadJSON,
			CreatedAt:      tx.createdAt,
			UpdatedAt:      tx.updatedAt,
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = record.UpdatedAt
		}

		if !recordMatchesReportFilter(record, filter) {
			continue
		}
		records = append(records, record)
	}

	sortReportRecords(records)
	if len(records) > filter.Limit {
		records = records[:filter.Limit]
	}
	return buildTransactionReport(filter, records), nil
}

func recordMatchesReportFilter(record storedReportTx, filter dto.TransactionReportFilter) bool {
	if filter.DateFrom != nil && record.CreatedAt.Before(*filter.DateFrom) {
		return false
	}
	if filter.DateTo != nil && record.CreatedAt.After(*filter.DateTo) {
		return false
	}
	if filter.Status != "" && !strings.EqualFold(record.Status, filter.Status) {
		return false
	}

	resp := paymentResponseFromStoredRecord(record)
	if filter.PaymentSystem != "" && !strings.EqualFold(resp.TransactionDetails.PaymentSystem, filter.PaymentSystem) {
		return false
	}
	if filter.PaymentMethod != "" && resp.PaymentInfo.PaymentMethodData.Type != filter.PaymentMethod {
		return false
	}
	return true
}

func buildTransactionReport(filter dto.TransactionReportFilter, records []storedReportTx) dto.TransactionReport {
	report := dto.TransactionReport{
		MerchantID:   filter.MerchantID,
		GeneratedAt:  time.Now().UTC(),
		Filter:       transactionReportFilterResponse(filter),
		Transactions: make([]dto.TransactionReportItem, 0, len(records)),
		Summary: dto.TransactionReportSummary{
			ByStatus:        map[string]dto.TransactionReportBucket{},
			ByPaymentSystem: map[string]dto.TransactionReportBucket{},
			ByPaymentMethod: map[string]dto.TransactionReportBucket{},
		},
	}

	for _, record := range records {
		resp := paymentResponseFromStoredRecord(record)
		item := transactionReportItem(record, resp)
		report.Transactions = append(report.Transactions, item)
		addTransactionToSummary(&report.Summary, item)
	}

	if report.Summary.TotalCount > 0 {
		report.Summary.AverageAmount = report.Summary.TotalAmount / float64(report.Summary.TotalCount)
	}
	return report
}

func paymentResponseFromStoredRecord(record storedReportTx) dto.PaymentResponse {
	payload := dto.SanitizePaymentPayloadJSON(record.PayloadJSON)
	var resp dto.PaymentResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return dto.PaymentResponse{
			ID:             record.PaymentID,
			MerchantID:     record.MerchantID,
			IdempotencyKey: record.IdempotencyKey,
			CurrentStatus:  record.Status,
		}
	}
	return resp.Sanitized()
}

func transactionReportItem(record storedReportTx, resp dto.PaymentResponse) dto.TransactionReportItem {
	status := firstNonEmptyString(resp.CurrentStatus, record.Status)
	paymentID := firstNonEmptyString(resp.ID, record.PaymentID)
	idempotencyKey := firstNonEmptyString(resp.IdempotencyKey, record.IdempotencyKey)

	return dto.TransactionReportItem{
		PaymentID:             paymentID,
		IdempotencyKey:        idempotencyKey,
		Status:                status,
		Amount:                resp.PaymentInfo.Amount,
		PaymentMethod:         resp.PaymentInfo.PaymentMethodData.Type,
		PaymentSystem:         resp.TransactionDetails.PaymentSystem,
		ProviderStatus:        resp.TransactionDetails.ProviderStatus,
		ExternalTransactionID: resp.TransactionDetails.ExternalTransactionID,
		FraudCheckResult:      resp.TransactionDetails.FraudCheckResult,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func addTransactionToSummary(summary *dto.TransactionReportSummary, item dto.TransactionReportItem) {
	amount := item.Amount.Value
	status := firstNonEmptyString(item.Status, "UNKNOWN")
	paymentSystem := firstNonEmptyString(item.PaymentSystem, "UNKNOWN")
	paymentMethod := firstNonEmptyString(string(item.PaymentMethod), "UNKNOWN")

	summary.TotalCount++
	summary.TotalAmount += amount

	switch strings.ToUpper(status) {
	case string(dto.StatusCaptured):
		summary.CapturedCount++
		summary.CapturedAmount += amount
	case string(dto.StatusPending), string(dto.StatusCaptureRequested):
		summary.PendingCount++
	case string(dto.StatusDeclined), string(dto.StatusCancelled):
		summary.DeclinedCount++
	case string(dto.StatusFailed):
		summary.FailedCount++
	}

	incrementReportBucket(summary.ByStatus, status, amount)
	incrementReportBucket(summary.ByPaymentSystem, paymentSystem, amount)
	incrementReportBucket(summary.ByPaymentMethod, paymentMethod, amount)
}

func incrementReportBucket(buckets map[string]dto.TransactionReportBucket, key string, amount float64) {
	bucket := buckets[key]
	bucket.Count++
	bucket.Amount += amount
	buckets[key] = bucket
}

func normalizeTransactionReportFilter(filter dto.TransactionReportFilter) dto.TransactionReportFilter {
	filter.MerchantID = strings.TrimSpace(filter.MerchantID)
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.PaymentSystem = strings.ToUpper(strings.TrimSpace(filter.PaymentSystem))
	filter.PaymentMethod = dto.PaymentMethodType(strings.TrimSpace(string(filter.PaymentMethod)))
	if filter.Limit <= 0 {
		filter.Limit = defaultReportLimit
	}
	if filter.Limit > maxReportLimit {
		filter.Limit = maxReportLimit
	}
	return filter
}

func transactionReportFilterResponse(filter dto.TransactionReportFilter) dto.TransactionReportFilterResponse {
	resp := dto.TransactionReportFilterResponse{
		MerchantID:    filter.MerchantID,
		Status:        filter.Status,
		PaymentSystem: filter.PaymentSystem,
		PaymentMethod: string(filter.PaymentMethod),
		Limit:         filter.Limit,
	}
	if filter.DateFrom != nil {
		resp.DateFrom = filter.DateFrom.Format(time.RFC3339)
	}
	if filter.DateTo != nil {
		resp.DateTo = filter.DateTo.Format(time.RFC3339)
	}
	return resp
}

func sortReportRecords(records []storedReportTx) {
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if records[j].CreatedAt.After(records[i].CreatedAt) {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
