package app

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"control/internal/types"
)

const (
	defaultDebugFormatQueueSize = 512
	defaultDebugFormatMaxBytes  = 32 * 1024
)

type DebugLineFormatter interface {
	Format(line string) (formatted string, changed bool)
}

type JSONDebugLineFormatter struct {
	MaxBytes int
}

func (f JSONDebugLineFormatter) Format(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line, false
	}
	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultDebugFormatMaxBytes
	}
	if len(trimmed) > maxBytes {
		return line, false
	}
	first := trimmed[0]
	if first != '{' && first != '[' {
		return line, false
	}
	raw := []byte(trimmed)
	if !json.Valid(raw) {
		return line, false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return line, false
	}
	formatted := buf.String()
	if formatted == line {
		return line, false
	}
	return formatted, true
}

type DebugStreamEntry struct {
	ID      string
	Seq     uint64
	Stream  string
	TS      string
	Raw     string
	Display string
}

type debugFormatRequest struct {
	generation uint64
	entryID    uint64
	line       string
}

type debugFormatResult struct {
	generation uint64
	entryID    uint64
	formatted  string
	changed    bool
}

type DebugFormatWorker interface {
	Enqueue(req debugFormatRequest)
	Drain(apply func(debugFormatResult))
	Close()
}

type asyncDebugFormatWorker struct {
	formatter DebugLineFormatter
	requests  chan debugFormatRequest
	results   chan debugFormatResult
	done      chan struct{}
	once      sync.Once
	wg        sync.WaitGroup
}

func newAsyncDebugFormatWorker(formatter DebugLineFormatter, queueSize int) *asyncDebugFormatWorker {
	if formatter == nil {
		formatter = JSONDebugLineFormatter{MaxBytes: defaultDebugFormatMaxBytes}
	}
	if queueSize <= 0 {
		queueSize = defaultDebugFormatQueueSize
	}
	w := &asyncDebugFormatWorker{
		formatter: formatter,
		requests:  make(chan debugFormatRequest, queueSize),
		results:   make(chan debugFormatResult, queueSize),
		done:      make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

func (w *asyncDebugFormatWorker) run() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case req, ok := <-w.requests:
			if !ok {
				return
			}
			formatted, changed := w.formatter.Format(req.line)
			res := debugFormatResult{
				generation: req.generation,
				entryID:    req.entryID,
				formatted:  formatted,
				changed:    changed,
			}
			select {
			case <-w.done:
				return
			case w.results <- res:
			default:
			}
		}
	}
}

func (w *asyncDebugFormatWorker) Enqueue(req debugFormatRequest) {
	if w == nil {
		return
	}
	select {
	case <-w.done:
		return
	case w.requests <- req:
	default:
	}
}

func (w *asyncDebugFormatWorker) Drain(apply func(debugFormatResult)) {
	if w == nil || apply == nil {
		return
	}
	for {
		select {
		case res := <-w.results:
			apply(res)
		default:
			return
		}
	}
}

func (w *asyncDebugFormatWorker) Close() {
	if w == nil {
		return
	}
	w.once.Do(func() {
		close(w.done)
		w.wg.Wait()
	})
}

type DebugStreamControllerOptions struct {
	Formatter         DebugLineFormatter
	FormatterQueue    int
	FormatterMaxBytes int
	FormatWorker      DebugFormatWorker
}

type DebugStreamControllerOption func(*DebugStreamControllerOptions)

func WithDebugLineFormatter(formatter DebugLineFormatter) DebugStreamControllerOption {
	return func(opts *DebugStreamControllerOptions) {
		opts.Formatter = formatter
	}
}

func WithDebugFormatQueueSize(queueSize int) DebugStreamControllerOption {
	return func(opts *DebugStreamControllerOptions) {
		opts.FormatterQueue = queueSize
	}
}

func WithDebugFormatMaxBytes(maxBytes int) DebugStreamControllerOption {
	return func(opts *DebugStreamControllerOptions) {
		opts.FormatterMaxBytes = maxBytes
	}
}

func WithDebugFormatWorker(worker DebugFormatWorker) DebugStreamControllerOption {
	return func(opts *DebugStreamControllerOptions) {
		opts.FormatWorker = worker
	}
}

func defaultDebugStreamControllerOptions() DebugStreamControllerOptions {
	return DebugStreamControllerOptions{
		FormatterQueue:    defaultDebugFormatQueueSize,
		FormatterMaxBytes: defaultDebugFormatMaxBytes,
	}
}

type DebugStreamController struct {
	events           <-chan types.DebugEvent
	cancel           func()
	entries          []DebugStreamEntry
	entryIDs         []uint64
	entryBytes       []int
	totalBytes       int
	contentCache     string
	contentDirty     bool
	retention        DebugStreamRetentionPolicy
	maxEventsPerTick int
	worker           DebugFormatWorker
	nextEntryID      uint64
	generation       uint64
	closed           bool
}

func NewDebugStreamController(retention DebugStreamRetentionPolicy, maxEventsPerTick int, options ...DebugStreamControllerOption) *DebugStreamController {
	retention = retention.normalize()
	opts := defaultDebugStreamControllerOptions()
	for _, apply := range options {
		if apply != nil {
			apply(&opts)
		}
	}
	if opts.Formatter == nil {
		opts.Formatter = JSONDebugLineFormatter{MaxBytes: opts.FormatterMaxBytes}
	}
	worker := opts.FormatWorker
	if worker == nil {
		worker = newAsyncDebugFormatWorker(opts.Formatter, opts.FormatterQueue)
	}
	return &DebugStreamController{
		retention:        retention,
		maxEventsPerTick: maxEventsPerTick,
		contentDirty:     true,
		worker:           worker,
	}
}

func (c *DebugStreamController) Close() {
	if c == nil || c.closed {
		return
	}
	c.Reset()
	if c.worker != nil {
		c.worker.Close()
	}
	c.closed = true
}

func (c *DebugStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = nil
	c.events = nil
	c.entries = nil
	c.entryIDs = nil
	c.entryBytes = nil
	c.totalBytes = 0
	c.contentCache = ""
	c.contentDirty = true
	c.generation++
}

func (c *DebugStreamController) SetStream(ch <-chan types.DebugEvent, cancel func()) {
	if c == nil {
		return
	}
	c.events = ch
	c.cancel = cancel
}

func (c *DebugStreamController) Lines() []string {
	if c == nil {
		return nil
	}
	c.drainFormatResults()
	lines := make([]string, 0, len(c.entries))
	for _, entry := range c.entries {
		lines = append(lines, entry.Display)
	}
	return lines
}

func (c *DebugStreamController) Entries() []DebugStreamEntry {
	if c == nil {
		return nil
	}
	c.drainFormatResults()
	entries := make([]DebugStreamEntry, len(c.entries))
	copy(entries, c.entries)
	return entries
}

func (c *DebugStreamController) HasStream() bool {
	return c != nil && c.events != nil
}

func (c *DebugStreamController) Content() string {
	if c == nil {
		return ""
	}
	c.drainFormatResults()
	if c.contentDirty {
		parts := make([]string, 0, len(c.entries))
		for _, entry := range c.entries {
			parts = append(parts, entry.Display)
		}
		c.contentCache = strings.Join(parts, "\n")
		c.contentDirty = false
	}
	return c.contentCache
}

func (c *DebugStreamController) ConsumeTick() (lines []string, changed bool, closed bool) {
	if c == nil {
		return nil, false, false
	}
	if c.drainFormatResults() {
		changed = true
	}
	if c.events == nil {
		return c.Lines(), changed, false
	}
	drain := true
	for i := 0; i < c.maxEventsPerTick && drain; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				closed = true
				drain = false
				break
			}
			if c.appendEvent(event) {
				changed = true
			}
		default:
			drain = false
		}
	}
	return c.Lines(), changed, closed
}

func (c *DebugStreamController) appendEvent(event types.DebugEvent) bool {
	if c == nil {
		return false
	}
	raw := strings.TrimRight(event.Chunk, "\n")
	if raw == "" {
		return false
	}
	c.nextEntryID++
	entryID := c.nextEntryID
	entry := DebugStreamEntry{
		ID:      debugEntryID(event, entryID),
		Seq:     event.Seq,
		Stream:  strings.TrimSpace(event.Stream),
		TS:      strings.TrimSpace(event.TS),
		Raw:     raw,
		Display: raw,
	}
	c.entries = append(c.entries, entry)
	c.entryIDs = append(c.entryIDs, entryID)
	eventBytes := len(raw)
	c.entryBytes = append(c.entryBytes, eventBytes)
	c.totalBytes += eventBytes
	c.enqueueFormat(entryID, raw)
	for {
		trimLines := c.retention.MaxLines > 0 && len(c.entries) > c.retention.MaxLines && len(c.entries) > 1
		trimBytes := c.retention.MaxBytes > 0 && c.totalBytes > c.retention.MaxBytes && len(c.entries) > 1
		if !trimLines && !trimBytes {
			break
		}
		if len(c.entries) == 0 {
			break
		}
		c.totalBytes -= c.entryBytes[0]
		c.entries = c.entries[1:]
		c.entryIDs = c.entryIDs[1:]
		c.entryBytes = c.entryBytes[1:]
	}
	c.contentDirty = true
	return true
}

func debugEntryID(event types.DebugEvent, entryID uint64) string {
	if event.Seq > 0 {
		return "debug-" + strconv.FormatUint(event.Seq, 10)
	}
	return "debug-entry-" + strconv.FormatUint(entryID, 10)
}

func (c *DebugStreamController) enqueueFormat(entryID uint64, line string) {
	if c == nil || c.worker == nil {
		return
	}
	c.worker.Enqueue(debugFormatRequest{
		generation: c.generation,
		entryID:    entryID,
		line:       line,
	})
}

func (c *DebugStreamController) drainFormatResults() bool {
	if c == nil || c.worker == nil {
		return false
	}
	changed := false
	c.worker.Drain(func(res debugFormatResult) {
		if !res.changed || res.generation != c.generation {
			return
		}
		for i, entryID := range c.entryIDs {
			if entryID != res.entryID {
				continue
			}
			if c.entries[i].Display == res.formatted {
				break
			}
			c.entries[i].Display = res.formatted
			c.contentDirty = true
			changed = true
			break
		}
	})
	return changed
}
