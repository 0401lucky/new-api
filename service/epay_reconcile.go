package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

type EpayOrderQueryResult struct {
	Code        int    `json:"code"`
	Message     string `json:"msg"`
	Status      int    `json:"status"`
	Pid         string `json:"pid"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Money       string `json:"money"`
	TradeNo     string `json:"trade_no"`
	OutTradeNo  string `json:"out_trade_no"`
	AddTime     string `json:"addtime"`
	EndTime     string `json:"endtime"`
	RawResponse string `json:"-"`
}

type EpayReconcileItem struct {
	TradeNo         string                `json:"trade_no"`
	UserId          int                   `json:"user_id"`
	Amount          int64                 `json:"amount"`
	Money           float64               `json:"money"`
	LocalStatus     string                `json:"local_status"`
	ProviderStatus  int                   `json:"provider_status"`
	ProviderTradeNo string                `json:"provider_trade_no"`
	ProviderMoney   string                `json:"provider_money"`
	Action          string                `json:"action"`
	Error           string                `json:"error,omitempty"`
	Query           *EpayOrderQueryResult `json:"query,omitempty"`
}

type EpayReconcileReport struct {
	Scanned   int                 `json:"scanned"`
	Queried   int                 `json:"queried"`
	Completed int                 `json:"completed"`
	Skipped   int                 `json:"skipped"`
	Failed    int                 `json:"failed"`
	DryRun    bool                `json:"dry_run"`
	Items     []EpayReconcileItem `json:"items"`
}

type EpayReconcileOptions struct {
	Limit         int
	MinAgeSeconds int64
	MaxAgeSeconds int64
	DryRun        bool
}

func ReconcilePendingEpayTopUps(opts EpayReconcileOptions) EpayReconcileReport {
	report := EpayReconcileReport{
		DryRun: opts.DryRun,
		Items:  make([]EpayReconcileItem, 0),
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	topUps, err := model.GetPendingEpayTopUps(opts.Limit, opts.MinAgeSeconds, opts.MaxAgeSeconds)
	if err != nil {
		report.Failed++
		report.Items = append(report.Items, EpayReconcileItem{
			Action: "query_local_failed",
			Error:  err.Error(),
		})
		return report
	}
	report.Scanned = len(topUps)
	for _, topUp := range topUps {
		if topUp == nil {
			continue
		}
		item := reconcileOneEpayTopUp(topUp, opts.DryRun)
		report.Items = append(report.Items, item)
		switch item.Action {
		case "completed":
			report.Completed++
		case "would_complete", "provider_pending", "provider_not_found", "money_mismatch", "pid_mismatch", "out_trade_no_mismatch":
			report.Skipped++
		default:
			if item.Error != "" {
				report.Failed++
			} else {
				report.Skipped++
			}
		}
		report.Queried++
	}
	return report
}

func reconcileOneEpayTopUp(topUp *model.TopUp, dryRun bool) EpayReconcileItem {
	item := EpayReconcileItem{
		TradeNo:     topUp.TradeNo,
		UserId:      topUp.UserId,
		Amount:      topUp.Amount,
		Money:       topUp.Money,
		LocalStatus: topUp.Status,
	}

	result, err := QueryEpayOrder(topUp.TradeNo)
	if err != nil {
		item.Action = "query_provider_failed"
		item.Error = err.Error()
		return item
	}
	item.Query = result
	item.ProviderStatus = result.Status
	item.ProviderTradeNo = result.TradeNo
	item.ProviderMoney = result.Money

	if result.Code != 1 {
		item.Action = "provider_not_found"
		item.Error = result.Message
		return item
	}
	if result.OutTradeNo != topUp.TradeNo {
		item.Action = "out_trade_no_mismatch"
		item.Error = fmt.Sprintf("provider out_trade_no=%s", result.OutTradeNo)
		return item
	}
	if result.Pid != operation_setting.EpayId {
		item.Action = "pid_mismatch"
		item.Error = "provider pid mismatch"
		return item
	}
	if !epayMoneyMatches(result.Money, topUp.Money) {
		item.Action = "money_mismatch"
		item.Error = fmt.Sprintf("provider money=%s local money=%.2f", result.Money, topUp.Money)
		return item
	}
	if result.Status != 1 {
		item.Action = "provider_pending"
		return item
	}
	if dryRun {
		item.Action = "would_complete"
		return item
	}

	if err := model.CompleteEpayTopUpByQuery(topUp.TradeNo, result.TradeNo, result.Type, result.Money); err != nil {
		item.Action = "complete_failed"
		item.Error = err.Error()
		return item
	}
	item.Action = "completed"
	return item
}

func QueryEpayOrder(outTradeNo string) (*EpayOrderQueryResult, error) {
	if outTradeNo == "" {
		return nil, errors.New("out_trade_no is empty")
	}
	if strings.TrimSpace(operation_setting.PayAddress) == "" ||
		strings.TrimSpace(operation_setting.EpayId) == "" ||
		strings.TrimSpace(operation_setting.EpayKey) == "" {
		return nil, errors.New("epay settings are incomplete")
	}

	endpoint, err := epayAPIEndpoint(operation_setting.PayAddress)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("act", "order")
	values.Set("pid", operation_setting.EpayId)
	values.Set("key", operation_setting.EpayKey)
	values.Set("out_trade_no", outTradeNo)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NewAPI-Epay-Reconcile/1.0")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("epay query http status=%d body=%s", resp.StatusCode, string(body))
	}

	var result EpayOrderQueryResult
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("epay query decode failed: %w body=%s", err, string(body))
	}
	result.RawResponse = string(body)
	return &result, nil
}

func epayAPIEndpoint(payAddress string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(payAddress))
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("invalid epay pay address")
	}

	path := strings.TrimRight(u.Path, "/")
	switch {
	case path == "" || path == "/":
		path = "/api.php"
	case strings.HasSuffix(path, "/api.php"):
		// keep as-is
	case strings.HasSuffix(path, "/pay"):
		path = strings.TrimSuffix(path, "/pay")
		path = strings.TrimRight(path, "/") + "/api.php"
	case strings.HasSuffix(path, "/submit.php"):
		path = strings.TrimSuffix(path, "/submit.php")
		path = strings.TrimRight(path, "/") + "/api.php"
	default:
		path = strings.TrimRight(path, "/") + "/api.php"
	}
	u.Path = path
	u.RawQuery = ""
	return u, nil
}

func epayMoneyMatches(providerMoney string, localMoney float64) bool {
	providerMoney = strings.TrimSpace(providerMoney)
	if providerMoney == "" {
		return false
	}
	parsed, err := decimal.NewFromString(providerMoney)
	if err != nil {
		return false
	}
	diff := parsed.Sub(decimal.NewFromFloat(localMoney)).Abs()
	return diff.LessThan(decimal.NewFromFloat(0.01))
}
