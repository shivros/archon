package app

import (
	"bytes"
	"encoding/json"
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

type debugFormatRequest struct {
	generation uint64
	lineID     uint64
	line       string
}

type debugFormatResult struct {
	generation uint64
	lineID     uint64
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
				lineID:     req.lineID,
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
	lines            []string
	displayLines     []string
	lineIDs          []uint64
	lineBytes        []int
	totalBytes       int
	pending          string
	contentCache     string
	contentDirty     bool
	retention        DebugStreamRetentionPolicy
	maxEventsPerTick int
	worker           DebugFormatWorker
	nextLineID       uint64
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
	c.lines = nil
	c.displayLines = nil
	c.lineIDs = nil
	c.lineBytes = nil
	c.totalBytes = 0
	c.pending = ""
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
	return c.displayLines
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
		c.contentCache = strings.Join(c.displayLines, "\n")
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
		return c.displayLines, changed, false
	}
	var builder strings.Builder
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
			builder.WriteString(event.Chunk)
		default:
			drain = false
		}
	}
	if builder.Len() > 0 {
		c.appendText(builder.String())
		changed = true
	}
	return c.displayLines, changed, closed
}

func (c *DebugStreamController) appendText(text string) {
	combined := c.pending + text
	parts := strings.Split(combined, "\n")
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		c.pending = parts[0]
		return
	}
	c.pending = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		c.nextLineID++
		lineID := c.nextLineID
		c.lines = append(c.lines, line)
		c.displayLines = append(c.displayLines, line)
		c.lineIDs = append(c.lineIDs, lineID)
		lineBytes := len(line)
		c.lineBytes = append(c.lineBytes, lineBytes)
		c.totalBytes += lineBytes
		c.enqueueFormat(lineID, line)
	}
	for {
		trimLines := c.retention.MaxLines > 0 && len(c.lines) > c.retention.MaxLines
		trimBytes := c.retention.MaxBytes > 0 && c.totalBytes > c.retention.MaxBytes
		if !trimLines && !trimBytes {
			break
		}
		if len(c.lines) == 0 {
			break
		}
		c.totalBytes -= c.lineBytes[0]
		c.lines = c.lines[1:]
		c.displayLines = c.displayLines[1:]
		c.lineIDs = c.lineIDs[1:]
		c.lineBytes = c.lineBytes[1:]
	}
	c.contentDirty = true
}

func (c *DebugStreamController) enqueueFormat(lineID uint64, line string) {
	if c == nil || c.worker == nil {
		return
	}
	c.worker.Enqueue(debugFormatRequest{
		generation: c.generation,
		lineID:     lineID,
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
		for i, lineID := range c.lineIDs {
			if lineID != res.lineID {
				continue
			}
			if c.displayLines[i] == res.formatted {
				break
			}
			c.displayLines[i] = res.formatted
			c.contentDirty = true
			changed = true
			break
		}
	})
	return changed
}
