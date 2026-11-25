package workbuddy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	freeWindowSpanMs    int64 = 24 * 3600 * 1000
	freeWindowLeadMs    int64 = 86400 * 1000
	freeWindowGapMs     int64 = freeWindowSpanMs
	routingLineMaxBytes       = 4 << 20
)

type freeWindowSpec struct {
	Provider    string
	Model       string
	Aliases     []string
	Cap         float64
	Name        string
	RefusedName string
	Short       string
	Unit        string
}

var (
	cnEstimatedWindows = []freeWindowSpec{
		{
			Provider:    ProviderWorkBuddy,
			Model:       "deepseek-v4.1-flash",
			Aliases:     []string{"deepseek-v4.1-flash-sg"},
			Cap:         2e8,
			Name:        "deepseek-v4.1-flash tokens (est.)",
			RefusedName: "deepseek-v4.1-flash tokens",
			Short:       "~200M",
			Unit:        "",
		},
	}
	freeWindowAI = freeWindowSpec{
		Provider: ProviderAI,
		Model:    "deepseek-v4.1-flash",
		Aliases:  []string{"deepseek-v4.1-flash-sg"},
		Cap:      1e8,
		Name:     "Free model tokens",
		Short:    "100M",
		Unit:     "tokens",
	}

	windowOffsetRe   = regexp.MustCompile(`(?i)\b(?:UTC|GMT)\s*([+-])\s*(\d{1,2})(?::?(\d{2}))?\b`)
	windowResetEnRe  = regexp.MustCompile(`reset at (\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})`)
	windowResetZhRe  = regexp.MustCompile(`将在\s*(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})`)
	routingSuffixRe  = regexp.MustCompile(`^(?:\.(\d+))?\.jsonl(\.gz)?$`)
	status429Re      = regexp.MustCompile(`"status"\s*:\s*429`)
	ledgerTimeRe     = regexp.MustCompile(`"t"\s*:\s*"([^"]{10,64})"`)
	ledgerAccountRe  = regexp.MustCompile(`"providerAccount"\s*:\s*"([^"]{0,256})"`)
	ledgerReportedRe = regexp.MustCompile(`"source"\s*:\s*"reported"`)
)

type usageRow struct {
	Provider        string `json:"provider"`
	ProviderAccount string `json:"providerAccount"`
	Model           string `json:"model"`
	T               string `json:"t"`
	Status          int    `json:"status"`
	In              int64  `json:"in"`
	CacheRead       int64  `json:"cache_read"`
	Out             int64  `json:"out"`
	Err             string `json:"err"`
	KeyedOnly       bool   `json:"-"`
	RestUntil       int64  `json:"-"`
}

type ledgerCacheEntry struct {
	Key  string
	Rows []usageRow
}

type ledgerBadState struct {
	Key   string
	Count int
}

type routingCacheEntry struct {
	Key         string
	Path        string
	Fingerprint string
	Size        int64
	Inode       uint64
	Offset      int64
	Rows        []usageRow
}

type routingReadFlight struct {
	done chan struct{}
	rows []usageRow
}

func fileInode(info os.FileInfo) uint64 {
	if info == nil {
		return 0
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat != nil {
		return uint64(stat.Ino)
	}
	return 0
}

func parseRowTimeMs(raw string) (int64, bool) {
	return parseISOMs(raw)
}

func windowOffset(text string) (int, bool) {
	match := windowOffsetRe.FindStringSubmatch(text)
	if len(match) < 3 {
		return 0, false
	}
	hours, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, false
	}
	mins := 0
	if len(match) >= 4 && match[3] != "" {
		parsedMins, err := strconv.Atoi(match[3])
		if err != nil {
			return 0, false
		}
		mins = parsedMins
	}
	total := hours*60 + mins
	if total > 14*60 {
		return 0, false
	}
	if match[1] == "-" {
		return -total, true
	}
	return total, true
}

func windowReset(text string) (int64, bool) {
	match := windowResetEnRe.FindStringSubmatch(text)
	if len(match) < 7 {
		match = windowResetZhRe.FindStringSubmatch(text)
	}
	if len(match) < 7 {
		return 0, false
	}
	offMin, ok := windowOffset(text)
	if !ok {
		return 0, false
	}
	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])
	hour, _ := strconv.Atoi(match[4])
	minute, _ := strconv.Atoi(match[5])
	second, _ := strconv.Atoi(match[6])
	utc := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return utc.UnixMilli() - int64(offMin)*60*1000, true
}

func resetOf(row usageRow) (int64, bool) {
	if row.RestUntil > 0 {
		return row.RestUntil, true
	}
	return windowReset(row.Err)
}

func windowModel(model string, win freeWindowSpec) bool {
	if model == "" {
		return false
	}
	id := model
	if idx := strings.IndexByte(model, '@'); idx >= 0 {
		id = model[:idx]
	}
	if id == win.Model {
		return true
	}
	return slices.Contains(win.Aliases, id)
}

func tokensShort(count float64) string {
	if count >= 1e6 {
		return fmt.Sprintf("%.1fM", count/1e6)
	}
	if count >= 1e3 {
		return fmt.Sprintf("%dk", int64(math.Round(count/1e3)))
	}
	return strconv.FormatInt(int64(math.Round(count)), 10)
}

func freeWindowCard(win freeWindowSpec, amount float64, resetsAtMs int64, refused bool) sdk.UsageWindow {
	models := make([]string, 0, 1+len(win.Aliases))
	models = append(models, win.Model)
	models = append(models, win.Aliases...)
	if refused {
		return sdk.UsageWindow{
			Name:     firstNonEmpty(win.RefusedName, win.Name),
			Used:     100,
			ResetsAt: resetsAtMs,
			Span:     freeWindowSpanMs / 1000,
			Models:   models,
			Aside:    false,
		}
	}
	amt := amount
	return sdk.UsageWindow{
		Name:     win.Name,
		Used:     clampPct((100 * amount) / win.Cap),
		Amount:   &amt,
		Limit:    win.Cap,
		Unit:     win.Unit,
		Display:  fmt.Sprintf("%s / %s", tokensShort(amount), win.Short),
		ResetsAt: resetsAtMs,
		Span:     freeWindowSpanMs / 1000,
		Models:   models,
		Aside:    true,
	}
}

func freeWindowOf(rows []usageRow, account string, nowMs int64, win freeWindowSpec) *sdk.UsageWindow {
	var mine []usageRow
	for _, row := range rows {
		if row.Provider == win.Provider && row.ProviderAccount == account && windowModel(row.Model, win) && row.T != "" {
			mine = append(mine, row)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	rowAt := func(row usageRow) int64 {
		ms, ok := parseRowTimeMs(row.T)
		if !ok {
			return 0
		}
		return ms
	}
	counted := func(row usageRow) float64 {
		return float64(row.In + row.CacheRead + row.Out)
	}
	apart := func(later, earlier int64) bool {
		return later-earlier >= freeWindowSpanMs
	}

	var edges []int64
	var live int64
	for _, row := range mine {
		if row.Status != http.StatusTooManyRequests {
			continue
		}
		resetMs, ok := resetOf(row)
		if !ok {
			continue
		}
		edges = append(edges, resetMs)
		whenMs := rowAt(row)
		if whenMs > 0 && whenMs <= nowMs && resetMs > nowMs && resetMs > live {
			live = resetMs
		}
	}
	if live > 0 {
		card := freeWindowCard(win, 0, live, true)
		return &card
	}

	sort.Slice(edges, func(left, right int) bool { return edges[left] < edges[right] })
	var answered []usageRow
	for _, row := range mine {
		if row.Status == http.StatusOK && rowAt(row) > 0 {
			answered = append(answered, row)
		}
	}
	if len(answered) == 0 {
		return nil
	}
	sort.Slice(answered, func(left, right int) bool { return rowAt(answered[left]) < rowAt(answered[right]) })

	head := func(from int) (int64, float64, bool) {
		var start int64
		hasStart := false
		var used float64
		edgeIdx := 0
		for idx := from; idx < len(answered); idx++ {
			row := answered[idx]
			ts := rowAt(row)
			for edgeIdx < len(edges) && edges[edgeIdx] <= ts {
				hasStart = false
				used = 0
				edgeIdx++
			}
			if !hasStart {
				start = ts
				hasStart = true
			} else if apart(ts, start) {
				start = ts
				used = 0
			}
			used += counted(row)
		}
		return start, used, hasStart
	}

	var foundStart int64
	var foundUsed float64
	found := false
	for idx := 0; idx < len(answered); idx++ {
		if idx > 0 && !apart(rowAt(answered[idx]), rowAt(answered[idx-1])) {
			continue
		}
		start, used, ok := head(idx)
		if !ok || start+freeWindowSpanMs <= nowMs {
			continue
		}
		foundStart = start
		foundUsed = used
		found = true
		break
	}
	if !found || foundStart <= 0 {
		return nil
	}
	end := foundStart + freeWindowSpanMs
	if nowMs >= end {
		return nil
	}
	card := freeWindowCard(win, foundUsed, end, false)
	return &card
}

func freeWindowLedger(cwd string, env func(string) string, home string) []string {
	var dirs []string
	if cwd != "" {
		dirs = append(dirs, cwd)
	}
	if env != nil {
		if xdg := env("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
			dirs = append(dirs, filepath.Join(xdg, "magpie"))
		}
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", "magpie"))
	}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, filepath.Join(dir, "usage.jsonl"))
	}
	return out
}

func freeWindowRouting(base string, nowMs int64) []string {
	dayOrder := make([]string, 0, 3)
	localNow := time.UnixMilli(nowMs).In(time.Local)
	for offset := 0; offset < 3; offset++ {
		day := localNow.AddDate(0, 0, -offset).Format("2006-01-02")
		dayOrder = append(dayOrder, day)
	}
	entries, err := os.ReadDir(filepath.Join(base, "routing"))
	if err != nil {
		return nil
	}
	var found []string
	for _, day := range dayOrder {
		type candidate struct {
			name string
			num  int
			gz   int
		}
		var dayFiles []candidate
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, day) {
				continue
			}
			match := routingSuffixRe.FindStringSubmatch(name[len(day):])
			if match == nil {
				continue
			}
			num := 0
			if match[1] != "" {
				num, _ = strconv.Atoi(match[1])
			}
			gzFlag := 0
			if match[2] != "" {
				gzFlag = 1
			}
			dayFiles = append(dayFiles, candidate{name: name, num: num, gz: gzFlag})
		}
		sort.Slice(dayFiles, func(left, right int) bool {
			if dayFiles[left].num != dayFiles[right].num {
				return dayFiles[left].num < dayFiles[right].num
			}
			return dayFiles[left].gz < dayFiles[right].gz
		})
		for _, item := range dayFiles {
			found = append(found, filepath.Join(base, "routing", item.name))
		}
	}
	return found
}

type restTarget struct {
	Account string
	Model   string
}

func restIdentity(rest map[string]any, win freeWindowSpec) (restTarget, bool) {
	if rest == nil {
		return restTarget{}, false
	}
	key, ok := rest["key"].(string)
	if !ok {
		return restTarget{}, false
	}
	prefix := win.Provider + "@"
	if !strings.HasPrefix(key, prefix) {
		return restTarget{}, false
	}
	tail := key[len(prefix):]
	modelAt := strings.LastIndexByte(tail, '/')
	if modelAt <= 0 || modelAt == len(tail)-1 {
		return restTarget{}, false
	}
	model := tail[modelAt+1:]
	if !windowModel(model, win) {
		return restTarget{}, false
	}
	return restTarget{Account: tail[:modelAt], Model: model}, true
}

func routingRefusals(win freeWindowSpec, text string) []usageRow {
	var rows []usageRow
	for _, line := range strings.Split(text, "\n") {
		routingRefusal(win, line, &rows)
	}
	return rows
}

func routingRefusal(win freeWindowSpec, line string, rows *[]usageRow) {
	if !status429Re.MatchString(line) || !strings.Contains(line, win.Provider) {
		return
	}
	var event map[string]any
	if json.Unmarshal([]byte(line), &event) != nil || event == nil {
		return
	}
	named := map[string]string{}
	ambiguous := map[string]bool{}
	if orders, ok := event["order"].([]any); ok {
		for _, rawOrder := range orders {
			order, ok := rawOrder.(map[string]any)
			if !ok || order == nil {
				continue
			}
			prov, hasProv := order["provider"].(string)
			orderID, hasID := order["id"].(string)
			who, hasWho := order["who"].(string)
			if !hasProv || prov == "" {
				prov = orderID
			}
			if prov != win.Provider {
				continue
			}
			if kind, ok := order["kind"].(string); ok && kind != "" && kind != "account" {
				continue
			}
			if !hasID || !hasWho {
				continue
			}
			if ambiguous[orderID] {
				continue
			}
			prev, exists := named[orderID]
			if !exists {
				named[orderID] = who
			} else if !strings.EqualFold(prev, who) {
				delete(named, orderID)
				ambiguous[orderID] = true
			}
		}
	}
	eventTime := strField(event, "time")
	tries, ok := event["tries"].([]any)
	if !ok {
		return
	}
	for _, rawAttempt := range tries {
		attempt, ok := rawAttempt.(map[string]any)
		if !ok || attempt == nil {
			continue
		}
		if parseIntCode(attempt["status"]) != http.StatusTooManyRequests {
			continue
		}
		if rawProv, exists := attempt["provider"]; exists && rawProv != nil {
			if provStr, isStr := rawProv.(string); !isStr || provStr != win.Provider {
				continue
			}
		}
		model := strField(attempt, "model")
		if !windowModel(model, win) {
			continue
		}
		errMsg := ""
		switch errVal := attempt["error"].(type) {
		case string:
			errMsg = errVal
		case map[string]any:
			if msg, ok := errVal["message"].(string); ok {
				errMsg = msg
			}
		}
		restMap, _ := attempt["rest"].(map[string]any)
		keyed, hasKeyed := restIdentity(restMap, win)
		errResetMs, hasErrReset := windowReset(errMsg)
		resetMs := errResetMs
		hasReset := hasErrReset
		var restUntil int64
		if !hasReset && hasKeyed && strField(restMap, "by") == "resets" {
			if untilStr, ok := restMap["until"].(string); ok {
				if untilMs, ok := parseISOMs(untilStr); ok && untilMs > 0 {
					restUntil = untilMs
					resetMs = untilMs
					hasReset = true
				}
			}
		}
		if !hasReset || resetMs <= 0 {
			continue
		}
		attemptID := ""
		if attempt["id"] != nil {
			attemptID = fmt.Sprint(attempt["id"])
		}
		if ambiguous[attemptID] {
			continue
		}
		ordered, hasOrdered := named[attemptID]
		if hasOrdered && hasKeyed && !strings.EqualFold(ordered, keyed.Account) {
			continue
		}
		account := ordered
		if !hasOrdered && hasKeyed {
			account = keyed.Account
		}
		if account == "" {
			continue
		}
		row := usageRow{
			Provider:        win.Provider,
			ProviderAccount: account,
			Model:           model,
			T:               firstNonEmpty(strField(attempt, "start"), eventTime),
			Status:          http.StatusTooManyRequests,
			Err:             errMsg,
			KeyedOnly:       !hasOrdered,
		}
		if !hasErrReset {
			row.RestUntil = restUntil
		}
		*rows = append(*rows, row)
	}
}

func routingLines(path string, compressed bool, fromOffset int64, each func(string)) int64 {
	file, err := os.Open(path)
	if err != nil {
		return fromOffset
	}
	defer file.Close()
	var reader io.Reader = file
	if compressed {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fromOffset
		}
		defer gzReader.Close()
		reader = gzReader
	} else if fromOffset > 0 {
		if _, err := file.Seek(fromOffset, io.SeekStart); err != nil {
			return fromOffset
		}
	}

	offset := fromOffset
	bufReader := bufio.NewReaderSize(reader, 64*1024)
	var lineBuf bytes.Buffer
	lineBytes := 0
	tooLong := false
	flushLine := func() {
		if !tooLong && lineBytes > 0 {
			each(lineBuf.String())
		}
		offset += int64(lineBytes)
		lineBuf.Reset()
		lineBytes = 0
		tooLong = false
	}
	chunk := make([]byte, 32*1024)
	for {
		count, readErr := bufReader.Read(chunk)
		if count > 0 {
			data := chunk[:count]
			start := 0
			for {
				nl := bytes.IndexByte(data[start:], '\n')
				if nl < 0 {
					break
				}
				piece := data[start : start+nl]
				if !tooLong {
					if lineBytes+len(piece) <= routingLineMaxBytes {
						lineBuf.Write(piece)
					} else {
						tooLong = true
						lineBuf.Reset()
					}
				}
				lineBytes += len(piece)
				flushLine()
				offset++
				start += nl + 1
			}
			if start < len(data) {
				piece := data[start:]
				if !tooLong {
					if lineBytes+len(piece) <= routingLineMaxBytes {
						lineBuf.Write(piece)
					} else {
						tooLong = true
						lineBuf.Reset()
					}
				}
				lineBytes += len(piece)
			}
		}
		if readErr == io.EOF {
			if compressed {
				flushLine()
			}
			break
		}
		if readErr != nil {
			break
		}
	}
	return offset
}

func (owner *module) readRoutingRefusals(path string, compressed bool, win freeWindowSpec) []usageRow {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%s|%s", path, win.Provider, win.Model)
	fingerprint := fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())

	owner.routingMu.Lock()
	held := owner.routingParsed[key]
	if held != nil && held.Fingerprint == fingerprint {
		rows := slices.Clone(held.Rows)
		owner.routingMu.Unlock()
		return rows
	}
	if flight, running := owner.routingReading[key]; running {
		owner.routingMu.Unlock()
		<-flight.done
		return slices.Clone(flight.rows)
	}
	flight := &routingReadFlight{done: make(chan struct{})}
	owner.routingReading[key] = flight
	owner.routingMu.Unlock()

	atomic.AddInt64(&owner.routingReads, 1)
	inode := fileInode(info)
	grown := !compressed && held != nil && held.Inode == inode && inode != 0 && info.Size() > held.Size
	var rows []usageRow
	var startOffset int64
	if grown {
		rows = slices.Clone(held.Rows)
		startOffset = held.Offset
	}
	nextOffset := routingLines(path, compressed, startOffset, func(line string) {
		routingRefusal(win, line, &rows)
	})
	entry := &routingCacheEntry{
		Key:         key,
		Path:        path,
		Fingerprint: fingerprint,
		Size:        info.Size(),
		Inode:       inode,
		Offset:      nextOffset,
		Rows:        slices.Clone(rows),
	}

	owner.routingMu.Lock()
	owner.routingParsed[key] = entry
	flight.rows = slices.Clone(rows)
	close(flight.done)
	delete(owner.routingReading, key)
	owner.routingMu.Unlock()
	return rows
}

func (owner *module) routingRefusalRows(path string, compressed bool, nowMs int64, account string, win freeWindowSpec) []usageRow {
	parsed := owner.readRoutingRefusals(path, compressed, win)
	return filterRoutingRefusalRows(parsed, nowMs, account)
}

func filterRoutingRefusalRows(parsed []usageRow, nowMs int64, account string) []usageRow {
	var rows []usageRow
	oldest := nowMs - 2*freeWindowSpanMs
	for _, attempt := range parsed {
		ts, ok := parseRowTimeMs(attempt.T)
		if !ok || ts < oldest || ts > nowMs {
			continue
		}
		if !strings.EqualFold(attempt.ProviderAccount, account) {
			continue
		}
		copied := attempt
		if copied.KeyedOnly {
			copied.ProviderAccount = account
		}
		rows = append(rows, copied)
	}
	return rows
}

func rowTime(line string) (int64, bool) {
	match := ledgerTimeRe.FindStringSubmatch(line)
	if len(match) < 2 {
		return 0, false
	}
	return parseRowTimeMs(match[1])
}

func rowReported(line string) bool {
	return ledgerReportedRe.MatchString(line)
}

func rowAccount(line string) string {
	match := ledgerAccountRe.FindStringSubmatch(line)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func ofAccount(row usageRow, account string, win freeWindowSpec) bool {
	if row.Provider != win.Provider || row.ProviderAccount != account || !windowModel(row.Model, win) || row.T == "" {
		return false
	}
	return row.Status == http.StatusOK || row.Status == http.StatusTooManyRequests
}

func newPinningTracker() func(usageRow) int64 {
	hasDenied := false
	var denied int64
	hasOldest := false
	var oldest int64
	return func(row usageRow) int64 {
		if row.Status == http.StatusTooManyRequests {
			if !hasDenied {
				if resetMs, ok := resetOf(row); ok {
					denied = resetMs
					hasDenied = true
				}
			}
			return 0
		}
		if hasDenied {
			return denied - freeWindowLeadMs
		}
		if row.Status != http.StatusOK {
			return 0
		}
		ts, ok := parseRowTimeMs(row.T)
		if !ok {
			return 0
		}
		if hasOldest && oldest-ts >= freeWindowGapMs {
			return ts - freeWindowLeadMs
		}
		oldest = ts
		hasOldest = true
		return 0
	}
}

type ledgerReadResult struct {
	Rows []usageRow
	Bad  int
}

func readLedgerTail(path string, info os.FileInfo, nowMs int64, onChunk func(), account string, win freeWindowSpec) (ledgerReadResult, error) {
	const chunkSize int64 = 1 << 16
	lapsed := nowMs - freeWindowSpanMs
	file, err := os.Open(path)
	if err != nil {
		return ledgerReadResult{}, err
	}
	defer file.Close()

	size := info.Size()
	tornEnd := false
	if size > 0 {
		one := make([]byte, 1)
		if _, err := file.ReadAt(one, size-1); err == nil {
			tornEnd = one[0] != '\n'
		}
	}
	var rev []usageRow
	bad := 0
	pos := size
	carry := ""
	firstPass := true
	stop := false
	edge := newPinningTracker()
	var reach int64
	answered := false

	for pos > 0 && !stop {
		readLen := chunkSize
		if pos < readLen {
			readLen = pos
		}
		pos -= readLen
		buf := make([]byte, readLen)
		if _, err := file.ReadAt(buf, pos); err != nil && err != io.EOF {
			return ledgerReadResult{}, err
		}
		if onChunk != nil {
			onChunk()
		}
		lines := strings.Split(string(buf)+carry, "\n")
		carry = lines[0]
		for idx := len(lines) - 1; idx >= 1 && !stop; idx-- {
			line := lines[idx]
			if strings.TrimSpace(line) == "" {
				continue
			}
			ts, hasTime := rowTime(line)
			dated := hasTime && !rowReported(line)
			if dated && reach > 0 && ts < reach {
				stop = true
				break
			}
			if dated && reach == 0 && !answered && ts < lapsed {
				stop = true
				break
			}
			if !strings.Contains(line, account) || rowAccount(line) != account {
				continue
			}
			var row usageRow
			if json.Unmarshal([]byte(line), &row) != nil {
				if !(firstPass && tornEnd && idx == len(lines)-1) {
					bad++
				}
				continue
			}
			if !ofAccount(row, account, win) {
				continue
			}
			rev = append(rev, row)
			if row.Status == http.StatusOK {
				answered = true
			}
			if reach == 0 {
				reach = edge(row)
			}
		}
		firstPass = false
	}
	if !stop && pos == 0 && strings.TrimSpace(carry) != "" && strings.Contains(carry, account) && rowAccount(carry) == account {
		ts, hasTime := rowTime(carry)
		if reach == 0 || !hasTime || ts >= reach {
			var row usageRow
			if json.Unmarshal([]byte(carry), &row) != nil {
				bad++
			} else if ofAccount(row, account, win) {
				rev = append(rev, row)
			}
		}
	}
	slices.Reverse(rev)
	return ledgerReadResult{Rows: rev, Bad: bad}, nil
}

func (owner *module) ledgerRows(path string, info os.FileInfo, nowMs int64, account string, win freeWindowSpec) []usageRow {
	fileKey := fmt.Sprintf("%s|%d:%d", path, info.ModTime().UnixNano(), info.Size())
	cacheKey := fmt.Sprintf("%s|%s|%s|%s", fileKey, win.Provider, win.Model, account)
	owner.ledgerMu.Lock()
	if owner.ledgerParsed != nil && owner.ledgerParsed.Key == cacheKey {
		rows := slices.Clone(owner.ledgerParsed.Rows)
		owner.ledgerMu.Unlock()
		return rows
	}
	owner.ledgerMu.Unlock()

	got, err := readLedgerTail(path, info, nowMs, nil, account, win)
	if err != nil {
		return nil
	}
	owner.ledgerMu.Lock()
	if got.Bad > 0 && owner.ledgerBad.Key != fileKey {
		owner.ledgerBad = ledgerBadState{Key: fileKey, Count: got.Bad}
	}
	owner.ledgerParsed = &ledgerCacheEntry{Key: cacheKey, Rows: slices.Clone(got.Rows)}
	owner.ledgerMu.Unlock()
	return got.Rows
}

func (owner *module) freeWindowRows(account string, nowMs int64, cwd string, env func(string) string, home string, win freeWindowSpec) []usageRow {
	if account == "" {
		return nil
	}
	for _, path := range freeWindowLedger(cwd, env, home) {
		info, err := os.Stat(path)
		hasRows := err == nil && !info.IsDir() && info.Size() > 0
		routes := freeWindowRouting(filepath.Dir(path), nowMs)
		if !hasRows && len(routes) == 0 {
			continue
		}
		var rows []usageRow
		if hasRows {
			rows = owner.ledgerRows(path, info, nowMs, account, win)
		}
		currentSet := map[string]bool{}
		for _, route := range routes {
			currentSet[route] = true
		}
		owner.routingMu.Lock()
		for key, held := range owner.routingParsed {
			if !currentSet[held.Path] {
				delete(owner.routingParsed, key)
			}
		}
		owner.routingMu.Unlock()
		for _, route := range routes {
			rows = append(rows, owner.routingRefusalRows(route, strings.HasSuffix(route, ".gz"), nowMs, account, win)...)
		}
		sort.SliceStable(rows, func(left, right int) bool {
			leftMs, _ := parseRowTimeMs(rows[left].T)
			rightMs, _ := parseRowTimeMs(rows[right].T)
			return leftMs < rightMs
		})
		return rows
	}
	return nil
}
