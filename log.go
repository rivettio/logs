package logs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

)

const (
	Ldate = 1 << iota
	Ltime
	Lmicroseconds
	Llongfile
	Lshortfile
	Lmodule
	Llevel
	LstdFlags = Ldate | Ltime
	Ldefault  = Llevel | Lshortfile | LstdFlags
)

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	PANIC
	FATAL
)

const LogSuffix = ".log"

var LevelNames = []string{
	"[DEBUG]",
	"[INFO ]",
	"[WARN ]",
	"[ERROR]",
	"[PANIC]",
	"[FATAL]",
}

var (
	console = &Logger{
		out:          os.Stdout,
		prefix:       "",
		level:        DEBUG,
		flag:         Ldefault,
		isConsole:    true,
		showFileLine: true,
	}

	std *Logger
)

type Logger struct {
	mu           MutexWrap
	prefix       string
	flag         int
	out          io.Writer
	buf          bytes.Buffer
	level        int
	levelStats   [6]int64
	logPath      string
	logFile      string
	create       time.Time
	isConsole    bool
	showFileLine bool
}

type MutexWrap struct {
	lock   sync.Mutex
	enable bool
}

func (mw *MutexWrap) Lock() {
	if mw.enable {
		mw.lock.Lock()
	}
}

func (mw *MutexWrap) Unlock() {
	if mw.enable {
		mw.lock.Unlock()
	}
}

func (mw *MutexWrap) Enable() {
	mw.enable = true
}

func Init(logPath string, logFile string, level int, isConsole bool, showFileLine bool, mutex bool) error {
	std = &Logger{
		prefix:       "",
		level:        level,
		flag:         Ldefault,
		logPath:      logPath,
		logFile:      logFile,
		create:       time.Now(),
		isConsole:    isConsole,
		showFileLine: showFileLine,
	}

	std.mu.enable = mutex

	file, err := os.OpenFile(std.logfileFullName(), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		console.Error(err)
		return err
	}

	std.out = file

	return nil
}

const DateFormat = "2006-01-02"

func (l *Logger) logfileName() string {
	return l.logFile + "." + l.create.Format(DateFormat) + LogSuffix
}

func (l *Logger) logfileFullName() string {
	return l.logPath + "/" + l.logfileName()
}

func itoa(buf *bytes.Buffer, i int, wid int) {
	var u = uint(i)
	if u == 0 && wid <= 1 {
		buf.WriteByte('0')
		return
	}

	var b [32]byte
	bp := len(b)
	for ; u > 0 || wid > 0; u /= 10 {
		bp--
		wid--
		b[bp] = byte(u%10) + '0'
	}

	for bp < len(b) {
		buf.WriteByte(b[bp])
		bp++
	}
}

func moduleOf(file string) string {
	pos := strings.LastIndex(file, "/")
	if pos != -1 {
		pos1 := strings.LastIndex(file[:pos], "/src/")
		if pos1 != -1 {
			return file[pos1+5 : pos]
		}
	}
	return "UNKNOWN"
}

func (l *Logger) formatHeader(buf *bytes.Buffer, t time.Time, file string, line int, lvl int) {
	if l.prefix != "" {
		buf.WriteString(l.prefix)
	}
	if l.flag&(Ldate|Ltime|Lmicroseconds) != 0 {
		if l.flag&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			buf.WriteByte('/')
			itoa(buf, int(month), 2)
			buf.WriteByte('/')
			itoa(buf, day, 2)
			buf.WriteByte(' ')
		}
		if l.flag&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			buf.WriteByte(':')
			itoa(buf, min, 2)
			buf.WriteByte(':')
			itoa(buf, sec, 2)
			if l.flag&Lmicroseconds != 0 {
				buf.WriteByte('.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			buf.WriteByte(' ')
		}
	}
	if l.flag&Llevel != 0 {
		buf.WriteString(LevelNames[lvl])
	}
	if l.flag&Lmodule != 0 {
		buf.WriteByte('[')
		buf.WriteString(moduleOf(file))
		buf.WriteByte(']')
		buf.WriteByte(' ')
	}
	if l.flag&(Lshortfile|Llongfile) != 0 {
		if l.flag&Lshortfile != 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}
		buf.WriteString(file)
		if line > 0 {
			buf.WriteByte(':')
			itoa(buf, line, -1)
		}
		buf.WriteString(": ")
	}
}

func (l *Logger) Error(e error) {

}

