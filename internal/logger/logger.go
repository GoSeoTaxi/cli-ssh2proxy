package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var L *zap.Logger

func Init(debug bool) {
	encCfg := zapcore.EncoderConfig{
		TimeKey:      "ts",
		LevelKey:     "lvl",
		MessageKey:   "msg",
		CallerKey:    "caller",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.LowercaseLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}
	enabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		if debug {
			return lvl >= zapcore.DebugLevel
		}
		switch lvl {
		case zapcore.InfoLevel, zapcore.PanicLevel, zapcore.DPanicLevel, zapcore.FatalLevel:
			return true
		default:
			return false
		}
	})
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		enabler,
	)
	L = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(L)
	zap.RedirectStdLog(L)
}
