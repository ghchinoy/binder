package okf

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrMissingFrontmatter = errors.New("missing frontmatter")
	ErrInvalidFrontmatter = errors.New("invalid frontmatter")
)

type Document struct {
	ConceptID   string
	Frontmatter map[string]any
	Order       []string
	Markdown    string
}

func IsReservedFile(name string) bool {
	base := path.Base(strings.TrimSpace(name))
	return base == "index.md" || base == "log.md"
}

func ConceptIDFromRel(rel string) (string, bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	if !strings.HasSuffix(rel, ".md") || IsReservedFile(rel) {
		return "", false
	}
	id := strings.TrimSuffix(rel, ".md")
	if id == "" || id == "." {
		return "", false
	}
	return id, true
}

func RelFromConceptID(id string) (string, error) {
	id = NormalizeConceptID(id)
	if id == "" {
		return "", fmt.Errorf("empty concept id")
	}
	for _, part := range strings.Split(id, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe concept id: %s", id)
		}
	}
	return id + ".md", nil
}

func NormalizeConceptID(id string) string {
	id = strings.TrimSpace(strings.ReplaceAll(id, "\\", "/"))
	id = strings.TrimPrefix(id, "/")
	id = path.Clean(id)
	if id == "." {
		return ""
	}
	id = strings.TrimSuffix(id, ".md")
	return id
}

func ParseConcept(conceptID string, data []byte) (Document, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	frontmatter, body, err := splitFrontmatter(text)
	if err != nil {
		return Document{}, err
	}
	values, order, err := ParseFrontmatter(frontmatter)
	if err != nil {
		return Document{}, err
	}
	return Document{
		ConceptID:   NormalizeConceptID(conceptID),
		Frontmatter: values,
		Order:       order,
		Markdown:    body,
	}, nil
}

func splitFrontmatter(text string) (string, string, error) {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return "", "", ErrMissingFrontmatter
	}
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimRight(line, "\r\n") == "---" {
			start := offset
			end := offset + len(line)
			return text[len(lines[0]):start], text[end:], nil
		}
		offset += len(line)
	}
	return "", "", ErrMissingFrontmatter
}

func ParseFrontmatter(text string) (map[string]any, []string, error) {
	values := map[string]any{}
	var order []string
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if indentation(raw) > 0 {
			return nil, nil, fmt.Errorf("%w: unexpected indentation on line %d", ErrInvalidFrontmatter, i+1)
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("%w: expected key-value pair on line %d", ErrInvalidFrontmatter, i+1)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, nil, fmt.Errorf("%w: empty key on line %d", ErrInvalidFrontmatter, i+1)
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		rawValue := strings.TrimSpace(parts[1])
		var value any
		var err error
		if rawValue == "|" || rawValue == ">" {
			var block []string
			block, i = collectIndentedBlock(lines, i+1)
			value = parseBlockScalar(block, rawValue == ">")
		} else if rawValue == "" && hasIndentedContent(lines, i+1) {
			var block []string
			block, i = collectIndentedBlock(lines, i+1)
			value, err = parseIndentedValue(block, i+1-len(block))
		} else {
			value, err = ParseValue(rawValue)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v on line %d", ErrInvalidFrontmatter, err, i+1)
		}
		values[key] = value
	}
	return values, order, nil
}

func ParseValue(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if text == "null" || text == "~" {
		return nil, nil
	}
	if strings.HasPrefix(text, "[") || strings.HasSuffix(text, "]") {
		if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
			return nil, fmt.Errorf("malformed list")
		}
		inside := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "["), "]"))
		if inside == "" {
			return []string{}, nil
		}
		parts := strings.Split(inside, ",")
		values := make([]any, 0, len(parts))
		allStrings := true
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item != "" {
				parsed, err := ParseValue(item)
				if err != nil {
					return nil, err
				}
				if _, ok := parsed.(string); !ok {
					allStrings = false
				}
				values = append(values, parsed)
			}
		}
		if allStrings {
			stringsOnly := make([]string, 0, len(values))
			for _, value := range values {
				stringsOnly = append(stringsOnly, value.(string))
			}
			return stringsOnly, nil
		}
		return values, nil
	}
	if (strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`)) || (strings.HasPrefix(text, `'`) && strings.HasSuffix(text, `'`)) {
		unquoted, err := strconv.Unquote(text)
		if err == nil {
			return unquoted, nil
		}
		return strings.Trim(text, `"'`), nil
	}
	if text == "true" {
		return true, nil
	}
	if text == "false" {
		return false, nil
	}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return f, nil
	}
	return text, nil
}

func Serialize(doc Document) []byte {
	body := strings.ReplaceAll(doc.Markdown, "\r\n", "\n")
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range orderedKeys(doc.Frontmatter, doc.Order) {
		writeFrontmatterEntry(&b, key, doc.Frontmatter[key], 0)
	}
	b.WriteString("---\n")
	if body != "" && !strings.HasPrefix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	return []byte(b.String())
}

func FormatValue(value any) string {
	switch v := value.(type) {
	case []string:
		return "[" + strings.Join(v, ", ") + "]"
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, strings.Trim(FormatValue(item), `"`))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		if v == "" {
			return `""`
		}
		if isPlainScalar(v) {
			return v
		}
		return strconv.Quote(v)
	default:
		return fmt.Sprint(v)
	}
}

func indentation(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' {
			count++
			continue
		}
		if r == '\t' {
			count += 2
			continue
		}
		break
	}
	return count
}

func hasIndentedContent(lines []string, start int) bool {
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		return indentation(line) > 0
	}
	return false
}

func collectIndentedBlock(lines []string, start int) ([]string, int) {
	var block []string
	i := start
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) != "" && indentation(line) == 0 {
			break
		}
		block = append(block, line)
	}
	return block, i - 1
}

func parseIndentedValue(lines []string, startLine int) (any, error) {
	trimmed := trimEmptyLines(lines)
	if len(trimmed) == 0 {
		return "", nil
	}
	minIndent := minNonEmptyIndent(trimmed)
	for i := range trimmed {
		if len(trimmed[i]) >= minIndent {
			trimmed[i] = trimmed[i][minIndent:]
		}
	}
	first := strings.TrimSpace(trimmed[0])
	if strings.HasPrefix(first, "- ") {
		return parseBlockList(trimmed, startLine)
	}
	return parseBlockMap(trimmed, startLine)
}

func trimEmptyLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return append([]string(nil), lines[start:end]...)
}

func minNonEmptyIndent(lines []string) int {
	min := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := indentation(line)
		if min == -1 || indent < min {
			min = indent
		}
	}
	if min < 0 {
		return 0
	}
	return min
}

func parseBlockMap(lines []string, startLine int) (map[string]any, error) {
	values := map[string]any{}
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		if indentation(raw) > 0 {
			return nil, fmt.Errorf("unexpected indentation on line %d", startLine+i)
		}
		key, rawValue, err := splitYAMLPair(strings.TrimSpace(raw), startLine+i)
		if err != nil {
			return nil, err
		}
		if rawValue == "|" || rawValue == ">" {
			var block []string
			block, i = collectIndentedBlock(lines, i+1)
			values[key] = parseBlockScalar(block, rawValue == ">")
			continue
		}
		if rawValue == "" && hasIndentedContent(lines, i+1) {
			var block []string
			block, i = collectIndentedBlock(lines, i+1)
			value, err := parseIndentedValue(block, startLine+i+1)
			if err != nil {
				return nil, err
			}
			values[key] = value
			continue
		}
		value, err := ParseValue(rawValue)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func parseBlockList(lines []string, startLine int) ([]any, error) {
	var values []any
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if indentation(raw) > 0 || !strings.HasPrefix(trimmed, "- ") {
			return nil, fmt.Errorf("expected list item on line %d", startLine+i)
		}
		item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if item == "" {
			var block []string
			block, i = collectIndentedBlock(lines, i+1)
			value, err := parseIndentedValue(block, startLine+i+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
			continue
		}
		if strings.Contains(item, ":") && !strings.HasPrefix(item, `"`) && !strings.HasPrefix(item, `'`) {
			key, rawValue, err := splitYAMLPair(item, startLine+i)
			if err == nil {
				m := map[string]any{}
				parsed, err := ParseValue(rawValue)
				if err != nil {
					return nil, err
				}
				m[key] = parsed
				values = append(values, m)
				continue
			}
		}
		parsed, err := ParseValue(item)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
	}
	return values, nil
}

func splitYAMLPair(line string, lineNumber int) (string, string, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key-value pair on line %d", lineNumber)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", fmt.Errorf("empty key on line %d", lineNumber)
	}
	return key, strings.TrimSpace(parts[1]), nil
}

func parseBlockScalar(lines []string, folded bool) string {
	trimmed := trimEmptyLines(lines)
	if len(trimmed) == 0 {
		return ""
	}
	minIndent := minNonEmptyIndent(trimmed)
	values := make([]string, 0, len(trimmed))
	for _, line := range trimmed {
		if len(line) >= minIndent {
			line = line[minIndent:]
		}
		values = append(values, strings.TrimRightFunc(line, unicode.IsSpace))
	}
	if folded {
		return strings.Join(values, " ")
	}
	return strings.Join(values, "\n") + "\n"
}

func writeFrontmatterEntry(b *strings.Builder, key string, value any, indent int) {
	prefix := strings.Repeat(" ", indent)
	switch v := value.(type) {
	case map[string]any:
		b.WriteString(prefix + key + ":\n")
		for _, child := range orderedKeys(v, nil) {
			writeFrontmatterEntry(b, child, v[child], indent+2)
		}
	case []any:
		if scalarList(v) {
			b.WriteString(prefix + key + ": " + FormatValue(v) + "\n")
			return
		}
		b.WriteString(prefix + key + ":\n")
		writeList(b, v, indent+2)
	default:
		b.WriteString(prefix + key + ": " + FormatValue(value) + "\n")
	}
}

func writeList(b *strings.Builder, values []any, indent int) {
	prefix := strings.Repeat(" ", indent)
	for _, value := range values {
		switch v := value.(type) {
		case map[string]any:
			b.WriteString(prefix + "-\n")
			for _, key := range orderedKeys(v, nil) {
				writeFrontmatterEntry(b, key, v[key], indent+2)
			}
		case []any:
			b.WriteString(prefix + "-\n")
			writeList(b, v, indent+2)
		default:
			b.WriteString(prefix + "- " + FormatValue(value) + "\n")
		}
	}
}

func scalarList(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func orderedKeys(values map[string]any, order []string) []string {
	seen := map[string]bool{}
	keys := make([]string, 0, len(values))
	for _, key := range order {
		if _, ok := values[key]; ok && !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range values {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func isPlainScalar(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	if strings.ContainsAny(value, "\n#{}[],&*?|-<>=!%@`") {
		return false
	}
	if strings.Contains(value, ": ") {
		return false
	}
	return true
}

func StringField(values map[string]any, key string) string {
	v, ok := values[key]
	if !ok {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func StringSliceField(values map[string]any, key string) []string {
	v, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return []string{fmt.Sprint(typed)}
	}
}
