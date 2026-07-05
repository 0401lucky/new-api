package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const (
	PromptCheckActionAllow = "allow"
	PromptCheckActionWarn  = "warn"
	PromptCheckActionBlock = "block"
)

type PromptCheckMatch struct {
	Name     string `json:"name"`
	Weight   int    `json:"weight"`
	Category string `json:"category,omitempty"`
	Strict   bool   `json:"strict,omitempty"`
}

type PromptCheckVerdict struct {
	Enabled         bool               `json:"enabled"`
	Mode            string             `json:"mode"`
	Action          string             `json:"action"`
	Score           int                `json:"score"`
	RawScore        int                `json:"raw_score"`
	Threshold       int                `json:"threshold"`
	StrictThreshold int                `json:"strict_threshold"`
	StrictHit       bool               `json:"strict_hit"`
	Matches         []PromptCheckMatch `json:"matches,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	TextPreview     string             `json:"text_preview,omitempty"`
	Reviewed        bool               `json:"reviewed,omitempty"`
	ReviewFlagged   bool               `json:"review_flagged,omitempty"`
	ReviewModel     string             `json:"review_model,omitempty"`
	ReviewError     string             `json:"review_error,omitempty"`
	ExtractedChars  int                `json:"extracted_chars"`
}

type promptCheckRule struct {
	Name     string
	Pattern  string
	Weight   int
	Category string
	Strict   bool
}

type compiledPromptCheckRule struct {
	promptCheckRule
	re *regexp.Regexp
}

var promptCheckRules = []promptCheckRule{
	{
		Name:     "prompt_injection_override",
		Pattern:  `(?i)\b(ignore|disregard|forget|override)\b.{0,80}\b(previous|above|system|developer|policy|safety|instruction|rules?)\b|忽略.{0,50}(之前|上面|系统|开发者|安全|规则|限制|指令)|覆盖.{0,40}(系统|开发者|安全|规则|限制|指令)`,
		Weight:   45,
		Category: "prompt_injection",
	},
	{
		Name:     "jailbreak_bypass",
		Pattern:  `(?i)\b(jailbreak|dan mode|developer mode|god mode|uncensored|unfiltered|no restrictions|bypass (?:policy|safety|guardrails)|break policy)\b|破限|越狱|绕过.{0,20}(安全|限制|审查|策略|规则)|无视.{0,20}(安全|限制|审查|策略|规则)`,
		Weight:   70,
		Category: "prompt_injection",
		Strict:   true,
	},
	{
		Name:     "system_prompt_extraction",
		Pattern:  `(?i)\b(reveal|print|dump|show|leak|extract)\b.{0,80}\b(system prompt|developer message|hidden instructions|internal policy|tool instructions|tools/instructions|skills/system|agents\.md|skill\.md)\b|(泄露|打印|输出|显示|提取).{0,60}(系统提示词|开发者消息|隐藏指令|内部策略|工具指令|AGENTS\.md|SKILL\.md)`,
		Weight:   75,
		Category: "prompt_extraction",
		Strict:   true,
	},
	{
		Name:     "credential_theft",
		Pattern:  `(?i)\b(steal|dump|extract|exfiltrate|harvest|grab)\b.{0,80}\b(credentials?|passwords?|tokens?|cookies?|api keys?|private keys?)\b|(?:写|生成|提供|构造|制作).{0,50}(窃取|导出|转储|提取).{0,40}(凭证|密码|令牌|token|cookie|密钥|私钥)`,
		Weight:   100,
		Category: "credential_theft",
		Strict:   true,
	},
	{
		Name:     "malware_or_stealer",
		Pattern:  `(?i)\b(keylogger|ransomware|trojan|backdoor|botnet|infostealer|credential stealer|c2 server|command and control)\b|键盘记录器|勒索软件|木马|后门|僵尸网络|窃密木马|C2服务器`,
		Weight:   85,
		Category: "malware",
		Strict:   true,
	},
	{
		Name:     "exploit_weaponization",
		Pattern:  `(?i)\b(write|generate|create|build|craft|implement|provide)\b.{0,100}\b(exploit|payload|shellcode|weaponized poc|0day|zero[-\s]?day|reverse shell|bind shell|web shell)\b|(?:写|生成|提供|构造|制作|实现).{0,60}(漏洞利用|攻击载荷|payload|shellcode|反弹\s*shell|web\s*shell|0day)`,
		Weight:   80,
		Category: "exploit",
		Strict:   true,
	},
	{
		Name:     "security_evasion",
		Pattern:  `(?i)\b(bypass|disable|evade|unhook)\b.{0,60}\b(av|edr|defender|antivirus|endpoint detection|anti[-\s]?debug|anti[-\s]?tamper|certificate pinning)\b|免杀|绕过.{0,40}(杀软|EDR|反调试|反篡改|证书绑定|证书固定)`,
		Weight:   80,
		Category: "evasion",
		Strict:   true,
	},
	{
		Name:     "reverse_engineering_abuse",
		Pattern:  `(?i)\b(ida pro|ghidra|x64dbg|ollydbg|frida|jadx|apktool|decompile|disassemble|reverse engineer)\b.{0,120}\b(bypass|crack|patch|remove|unlock|extract|dump|decrypt)\b.{0,80}\b(license|activation|trial|paywall|subscription|in[-\s]?app purchase|api keys?|tokens?|secrets?)\b|(?:ida|ghidra|x64dbg|frida|jadx|apktool|反编译|逆向|脱壳|调试).{0,80}(绕过|破解|补丁|去除|解锁|提取|导出|解密).{0,50}(授权|激活|试用|会员|订阅|付费|内购|密钥|token|令牌)`,
		Weight:   90,
		Category: "reverse_engineering",
		Strict:   true,
	},
	{
		Name:     "generic_reverse_engineering",
		Pattern:  `(?i)\b(ida pro|ghidra|x64dbg|ollydbg|frida hook|jadx|apktool|decompile|disassemble|reverse engineer)\b|反编译|逆向|脱壳|动态调试`,
		Weight:   18,
		Category: "reverse_engineering",
	},
	{
		Name:     "phishing_or_social_engineering",
		Pattern:  `(?i)\b(phishing page|phishing email|fake login|credential harvesting|spoof website|session hijacking|cookie theft)\b|钓鱼页面|钓鱼邮件|伪造登录|会话劫持|cookie窃取`,
		Weight:   80,
		Category: "social_engineering",
		Strict:   true,
	},
	{
		Name:     "nsfw_generation",
		Pattern:  `(?i)\b(nsfw|porn|pornographic|erotic|explicit sexual|nude|nudity|undress|sex scene|onlyfans)\b|色情|裸露|裸体|脱衣|成人电影|成人视频|性行为|淫秽`,
		Weight:   75,
		Category: "sexual",
		Strict:   true,
	},
	{
		Name:     "sexual_minors",
		Pattern:  `(?i)\b(child|minor|underage|teen)\b.{0,80}\b(sexual|nude|porn|erotic|explicit)\b|未成年.{0,40}(色情|裸露|性|淫秽)|儿童.{0,40}(色情|裸露|性|淫秽)`,
		Weight:   140,
		Category: "sexual_minors",
		Strict:   true,
	},
	{
		Name:     "high_risk_cyber_terms",
		Pattern:  `(?i)\b(mimikatz|cobalt strike|meterpreter|lsass dump|ntds\.dit|pass[-\s]?the[-\s]?hash|sql injection payload|rce exploit|kernel exploit|container escape)\b|凭证转储|横向移动|命令注入|SQL注入攻击|容器逃逸|内核提权`,
		Weight:   55,
		Category: "cyber",
	},
}

var compiledPromptCheckRules = compilePromptCheckRules(promptCheckRules)

var promptCheckRedactionRules = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(api[_ -]?key\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
}

type moderationRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}

type moderationResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Flagged bool `json:"flagged"`
	} `json:"results"`
}

func compilePromptCheckRules(rules []promptCheckRule) []compiledPromptCheckRule {
	compiled := make([]compiledPromptCheckRule, 0, len(rules))
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			common.SysError(fmt.Sprintf("compile prompt check rule %s failed: %s", rule.Name, err.Error()))
			continue
		}
		compiled = append(compiled, compiledPromptCheckRule{
			promptCheckRule: rule,
			re:              re,
		})
	}
	return compiled
}

func CheckPromptText(ctx context.Context, text string) PromptCheckVerdict {
	mode := setting.NormalizePromptCheckMode(setting.PromptCheckMode)
	threshold := setting.PromptCheckEffectiveThreshold()
	strictThreshold := setting.PromptCheckEffectiveStrictThreshold()
	text = limitPromptCheckText(text, setting.PromptCheckEffectiveMaxTextLength())
	verdict := PromptCheckVerdict{
		Enabled:         setting.ShouldCheckPromptSensitive(),
		Mode:            mode,
		Action:          PromptCheckActionAllow,
		Threshold:       threshold,
		StrictThreshold: strictThreshold,
		TextPreview:     PromptCheckRedactedPreview(text, 500),
		ExtractedChars:  utf8.RuneCountInString(text),
	}
	if !verdict.Enabled || strings.TrimSpace(text) == "" {
		return verdict
	}

	scanText := normalizePromptCheckText(text)
	matchesByName := make(map[string]PromptCheckMatch)
	rawScore := 0
	strictScore := 0

	for _, word := range setting.SensitiveWords {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" {
			continue
		}
		if strings.Contains(scanText, word) {
			key := "blocked_keyword:" + word
			matchesByName[key] = PromptCheckMatch{
				Name:     "blocked_keyword",
				Weight:   100,
				Category: "keyword",
				Strict:   true,
			}
		}
	}

	for _, rule := range compiledPromptCheckRules {
		if rule.re.MatchString(scanText) {
			matchesByName[rule.Name] = PromptCheckMatch{
				Name:     rule.Name,
				Weight:   rule.Weight,
				Category: rule.Category,
				Strict:   rule.Strict,
			}
		}
	}

	matches := make([]PromptCheckMatch, 0, len(matchesByName))
	for _, match := range matchesByName {
		matches = append(matches, match)
		rawScore += match.Weight
		if match.Strict {
			strictScore += match.Weight
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Weight == matches[j].Weight {
			return matches[i].Name < matches[j].Name
		}
		return matches[i].Weight > matches[j].Weight
	})

	score := rawScore
	if rawScore > 0 && strictScore == 0 {
		score -= promptCheckDefensiveContextDiscount(scanText)
		if score < 0 {
			score = 0
		}
	}

	strictHit := strictScore >= strictThreshold
	verdict.RawScore = rawScore
	verdict.Score = score
	verdict.StrictHit = strictHit
	verdict.Matches = matches
	if len(matches) == 0 {
		return verdict
	}

	if score >= threshold || strictHit {
		switch mode {
		case setting.PromptCheckModeBlock:
			verdict.Action = PromptCheckActionBlock
		case setting.PromptCheckModeWarn:
			verdict.Action = PromptCheckActionWarn
		default:
			verdict.Action = PromptCheckActionAllow
		}
	}
	verdict.Reason = promptCheckReason(verdict)

	if shouldReviewPromptCheck(verdict) {
		verdict = applyPromptCheckReview(ctx, text, verdict)
	}
	return verdict
}

func shouldReviewPromptCheck(verdict PromptCheckVerdict) bool {
	if verdict.Action != PromptCheckActionWarn && verdict.Action != PromptCheckActionBlock {
		return false
	}
	return setting.PromptCheckAPIReviewReady()
}

func applyPromptCheckReview(ctx context.Context, text string, verdict PromptCheckVerdict) PromptCheckVerdict {
	flagged, model, err := reviewPromptText(ctx, text)
	verdict.Reviewed = true
	verdict.ReviewFlagged = flagged
	verdict.ReviewModel = model
	if err != nil {
		verdict.ReviewError = err.Error()
		if setting.PromptCheckAPIReviewFailClosedEnabled {
			verdict.Action = PromptCheckActionBlock
			verdict.Reason = "prompt review failed; fail-closed policy blocked the request"
		} else {
			verdict.Action = PromptCheckActionAllow
			verdict.Reason = "prompt review failed; allowed by policy"
		}
		return verdict
	}
	if !flagged {
		verdict.Action = PromptCheckActionAllow
		verdict.Reason = "prompt review cleared local filter match"
		return verdict
	}
	verdict.Reason = "prompt review confirmed local filter match"
	return verdict
}

func reviewPromptText(ctx context.Context, text string) (bool, string, error) {
	endpoint, err := promptCheckReviewEndpoint(setting.PromptCheckAPIReviewBaseURL)
	if err != nil {
		return false, setting.PromptCheckAPIReviewModel, err
	}
	body, err := common.Marshal(moderationRequest{
		Model: setting.PromptCheckAPIReviewModel,
		Input: text,
	})
	if err != nil {
		return false, setting.PromptCheckAPIReviewModel, err
	}
	timeout := time.Duration(setting.PromptCheckEffectiveReviewTimeoutMS()) * time.Millisecond
	reviewCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reviewCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, setting.PromptCheckAPIReviewModel, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(setting.PromptCheckAPIReviewKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, setting.PromptCheckAPIReviewModel, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msgBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(msgBytes))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return false, setting.PromptCheckAPIReviewModel, fmt.Errorf("review request failed: status=%d, %s", resp.StatusCode, common.MaskSensitiveInfo(msg))
	}

	var decoded moderationResponse
	if err := common.DecodeJson(resp.Body, &decoded); err != nil {
		return false, setting.PromptCheckAPIReviewModel, err
	}
	if len(decoded.Results) == 0 {
		return false, setting.PromptCheckAPIReviewModel, errors.New("review response missing results")
	}
	flagged := false
	for _, result := range decoded.Results {
		if result.Flagged {
			flagged = true
			break
		}
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = setting.PromptCheckAPIReviewModel
	}
	return flagged, model, nil
}

func promptCheckReviewEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("review base URL must start with http:// or https://")
	}
	if strings.HasSuffix(parsed.Path, "/moderations") {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String(), nil
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		parsed.Path = path + "/moderations"
	} else {
		parsed.Path = path + "/v1/moderations"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizePromptCheckText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func limitPromptCheckText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	head := maxRunes * 4 / 5
	tail := maxRunes - head
	return string(runes[:head]) + "\n...[prompt check truncated]...\n" + string(runes[len(runes)-tail:])
}

func promptCheckDefensiveContextDiscount(text string) int {
	discount := 0
	defensiveTerms := []string{
		"ctf", "capture the flag", "authorized", "defensive", "blue team", "security review",
		"vulnerability report", "patch", "fix", "detect", "hardening", "training lab",
		"靶场", "授权", "防御", "蓝队", "安全审计", "漏洞报告", "修复", "检测", "加固", "课程", "教学", "实验",
	}
	for _, term := range defensiveTerms {
		if strings.Contains(text, term) {
			discount += 25
			if discount >= 50 {
				return 50
			}
		}
	}
	return discount
}

func promptCheckReason(verdict PromptCheckVerdict) string {
	if len(verdict.Matches) == 0 {
		return ""
	}
	top := verdict.Matches[0]
	return fmt.Sprintf("prompt check matched %s, score=%d, threshold=%d", top.Name, verdict.Score, verdict.Threshold)
}

func PromptCheckPreview(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func PromptCheckRedact(text string) string {
	redacted := text
	for _, re := range promptCheckRedactionRules {
		redacted = re.ReplaceAllString(redacted, "${1}[redacted]")
	}
	return redacted
}

func PromptCheckRedactedPreview(text string, maxRunes int) string {
	return PromptCheckPreview(PromptCheckRedact(text), maxRunes)
}
