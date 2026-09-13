package proxy

import (
	"bufio"
	"net"
	"strings"
	"sync"
)

// Rule 表示一条 MITM 域名规则
type Rule struct {
	raw        string
	isNeg      bool   // 是否否定规则（以 ! 开头）
	isWildcard bool   // 是否为 *.domain 形式
	isAll      bool   // 是否匹配全部（*）
	domain     string // 域名部分，不含 "*."
}

// RuleSet MITM 域名规则引擎
type RuleSet struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewRuleSet 创建规则集
func NewRuleSet() *RuleSet {
	return &RuleSet{}
}

// Load 从字符串加载规则（每行一条，支持 # 注释）
func (r *RuleSet) Load(rs string) error {
	reader := strings.NewReader(rs)
	scanner := bufio.NewScanner(reader)

	var rules []Rule
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		isNeg := false
		if strings.HasPrefix(line, "!") {
			isNeg = true
			line = strings.TrimSpace(line[1:])
			if line == "" {
				continue
			}
		}

		if line == "*" {
			rules = append(rules, Rule{
				raw:   "*",
				isAll: true,
				isNeg: isNeg,
			})
			continue
		}

		isWildcard := false
		domain := line
		if strings.HasPrefix(line, "*.") {
			isWildcard = true
			domain = line[2:]
		}

		rules = append(rules, Rule{
			raw:        line,
			isNeg:      isNeg,
			isWildcard: isWildcard,
			domain:     strings.ToLower(domain),
		})
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.rules = rules
	r.mu.Unlock()
	return nil
}

// ShouldMitm 根据当前规则集判断是否对 host 做 MITM
// host 可能带端口（example.com:443），函数会只匹配 hostname 部分
// 返回 true => MITM（解密），false => 透传
func (r *RuleSet) ShouldMitm(host string) bool {
	h := host
	if strings.HasPrefix(h, "[") {
		if idx := strings.LastIndex(h, "]"); idx != -1 {
			h = h[:idx+1]
		}
	}
	if hp, _, err := net.SplitHostPort(host); err == nil {
		h = hp
	}
	h = strings.ToLower(strings.Trim(h, "[]"))

	r.mu.RLock()
	defer r.mu.RUnlock()

	action := false
	for _, rule := range r.rules {
		if rule.isAll {
			action = !rule.isNeg
			continue
		}

		if rule.isWildcard {
			if h == rule.domain || strings.HasSuffix(h, "."+rule.domain) {
				action = !rule.isNeg
			}
			continue
		}

		if h == rule.domain {
			action = !rule.isNeg
		}
	}
	return action
}
