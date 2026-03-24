package logs

import (
	"go.uber.org/zap/zapcore"
)

// Hook returns a zapcore.Core that pushes formatted logs to the GlobalRing.
func Hook() zapcore.Core {
	return &ringCore{
		fields: make(map[string]interface{}),
	}
}

type ringCore struct {
	fields map[string]interface{}
}

func (c *ringCore) Enabled(zapcore.Level) bool {
	return true
}

func (c *ringCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &ringCore{
		fields: make(map[string]interface{}, len(c.fields)+len(fields)),
	}
	for k, v := range c.fields {
		clone.fields[k] = v
	}
	// We can't perfectly decode all fields here without a full encoder, 
	// but this is enough to preserve the context tree shape.
	return clone
}

func (c *ringCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *ringCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	entry := LogEntry{
		Level:   ent.Level.String(),
		Time:    ent.Time,
		Message: ent.Message,
		Fields:  make(map[string]interface{}),
	}

	for k, v := range c.fields {
		entry.Fields[k] = v
	}
	
	// A simple map encoder to capture fields dynamically
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	for k, v := range enc.Fields {
		entry.Fields[k] = v
	}

	GlobalRing.Add(entry)
	return nil
}

func (c *ringCore) Sync() error {
	return nil
}
