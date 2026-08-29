package eval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

func evalMath(body string) []Result {
	val, expr, err := evalExpr(body)
	if err != nil {
		return nil
	}
	out := formatNumber(val)
	sub := ""
	if strings.TrimSpace(expr) != out {
		sub = strings.TrimSpace(expr)
	}
	return []Result{{
		Kind:     KindMath,
		Title:    out,
		Subtitle: sub,
		Copy:     out,
		Rank:     90,
	}}
}

func evalExpr(s string) (float64, string, error) {
	toks, err := lexExpr(s)
	if err != nil {
		return 0, "", err
	}
	p := &parser{toks: toks}
	v, err := p.parseExpr()
	if err != nil {
		return 0, "", err
	}
	if p.i < len(p.toks) {
		return 0, "", fmt.Errorf("trailing input")
	}
	return v, prettyExpr(s), nil
}

type tkind int

const (
	tNum tkind = iota
	tOp
	tWord
)

type token struct {
	kind tkind
	num  float64
	op   string
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) peek() (token, bool) {
	if p.i >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.i], true
}

func (p *parser) take() (token, bool) {
	t, ok := p.peek()
	if ok {
		p.i++
	}
	return t, ok
}

func (p *parser) acceptOp(ops ...string) bool {
	t, ok := p.peek()
	if !ok || t.kind != tOp {
		return false
	}
	for _, op := range ops {
		if t.op == op {
			p.i++
			return true
		}
	}
	return false
}

func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tOp || (t.op != "+" && t.op != "-") {
			return left, nil
		}
		p.i++
		// A + B% means left ± left*(B/100)
		if pct, ok := p.parseBarePercent(); ok {
			if t.op == "+" {
				left += left * pct / 100
			} else {
				left -= left * pct / 100
			}
			continue
		}
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if t.op == "+" {
			left += right
		} else {
			left -= right
		}
	}
}

func (p *parser) parseBarePercent() (float64, bool) {
	if p.i+1 >= len(p.toks) {
		return 0, false
	}
	a, b := p.toks[p.i], p.toks[p.i+1]
	if a.kind == tNum && b.kind == tOp && b.op == "%" {
		// Do not steal "15% of X"
		if p.i+2 < len(p.toks) && p.toks[p.i+2].kind == tWord && p.toks[p.i+2].op == "of" {
			return 0, false
		}
		p.i += 2
		return a.num, true
	}
	return 0, false
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		if p.acceptOp("*", "×") {
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			left *= right
			continue
		}
		if p.acceptOp("/", "÷") {
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
			continue
		}
		return left, nil
	}
}

func (p *parser) parsePower() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	if p.acceptOp("^") {
		right, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func (p *parser) parseUnary() (float64, error) {
	if p.acceptOp("-", "−") {
		v, err := p.parseUnary()
		return -v, err
	}
	if p.acceptOp("+") {
		return p.parseUnary()
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() (float64, error) {
	v, err := p.parsePrimary()
	if err != nil {
		return 0, err
	}
	if p.acceptOp("%") {
		t, ok := p.peek()
		if ok && t.kind == tWord && t.op == "of" {
			p.i++
			of, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			return of * v / 100, nil
		}
		return v / 100, nil
	}
	return v, nil
}

func (p *parser) parsePrimary() (float64, error) {
	t, ok := p.take()
	if !ok {
		return 0, fmt.Errorf("expected value")
	}
	switch t.kind {
	case tNum:
		return t.num, nil
	case tWord:
		switch t.op {
		case "pi":
			return math.Pi, nil
		case "e":
			return math.E, nil
		}
		return 0, fmt.Errorf("unknown name %q", t.op)
	case tOp:
		if t.op == "(" {
			v, err := p.parseExpr()
			if err != nil {
				return 0, err
			}
			if !p.acceptOp(")") {
				return 0, fmt.Errorf("missing )")
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("expected value")
}

func lexExpr(s string) ([]token, error) {
	var toks []token
	i := 0
	rs := []rune(s)
	for i < len(rs) {
		r := rs[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if unicode.IsDigit(r) || (r == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])) {
			start := i
			i++
			for i < len(rs) {
				if unicode.IsDigit(rs[i]) || rs[i] == '.' || rs[i] == ',' {
					i++
					continue
				}
				break
			}
			raw := strings.ReplaceAll(string(rs[start:i]), ",", "")
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tNum, num: n})
			continue
		}
		if unicode.IsLetter(r) {
			start := i
			i++
			for i < len(rs) && unicode.IsLetter(rs[i]) {
				i++
			}
			w := strings.ToLower(string(rs[start:i]))
			toks = append(toks, token{kind: tWord, op: w})
			continue
		}
		op := string(r)
		switch r {
		case '×':
			op = "*"
		case '÷':
			op = "/"
		case '−':
			op = "-"
		}
		toks = append(toks, token{kind: tOp, op: op})
		i++
	}
	return toks, nil
}

func prettyExpr(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func formatNumber(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "∞"
	}
	if math.Abs(v-math.Round(v)) < 1e-9 && math.Abs(v) < 1e15 {
		return formatInt(int64(math.Round(v)))
	}
	prec := 6
	av := math.Abs(v)
	switch {
	case av >= 1000:
		prec = 2
	case av >= 1:
		prec = 4
	}
	s := strconv.FormatFloat(v, 'f', prec, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func formatInt(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.WriteString(sign)
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func formatMoney(v float64, currency string) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	parts := strings.SplitN(s, ".", 2)
	whole := parts[0]
	frac := "00"
	if len(parts) == 2 {
		frac = parts[1]
	}
	n, _ := strconv.ParseInt(whole, 10, 64)
	out := formatInt(n) + "." + frac
	cur := strings.ToUpper(currency)
	if cur == "USD" || cur == "" {
		out = "$" + out
	} else {
		out = out + " " + cur
	}
	if neg {
		return "-" + out
	}
	return out
}
