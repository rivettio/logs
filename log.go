package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
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

func (l *Logger) Output(level int, s string) error {
	if level < l.level {
		return nil
	}

	if err := l.checkFile(); err != nil {
		return err
	}

	now := time.Now() // get this early.
	var file string
	var line int
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.flag&(Lshortfile|Llongfile|Lmodule) != 0 && l.showFileLine {
		l.mu.Unlock()
		var ok bool
		_, file, line, ok = runtime.Caller(2)
		if !ok {
			file = "???"
			line = 0
		}
		l.mu.Lock()
	}
	l.levelStats[level]++
	l.buf.Reset()
	l.formatHeader(&l.buf, now, file, line, level)
	l.buf.WriteString(s)
	if len(s) > 0 && s[len(s)-1] != '\n' {
		l.buf.WriteByte('\n')
	}
	_, err := l.out.Write(l.buf.Bytes())
	return err
}

func (l *Logger) checkFile() error {
	now := time.Now()
	if (now.Year() > l.create.Year()) || (now.Month() > l.create.Month()) || (now.Day() > l.create.Day()) {
		l.mu.Lock()
		defer l.mu.Unlock()

		l.create = now

		if l.logPath != "" && l.logFile != "" {
			newOut, err := os.OpenFile(l.logfileFullName(), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
			if err != nil {
				console.Error(err)
				return err
			}

			// close old out
			if f, ok := l.out.(*os.File); ok {
				if err := f.Close(); err != nil {
					console.Error(err)
					return err
				}
			}

			l.out = newOut
		}
	}

	return nil
}

func (l *Logger) Printf(format string, v ...interface{}) {
	l.Output(INFO, fmt.Sprintf(format, v...))
}

func (l *Logger) Print(v ...interface{}) {
	l.Output(INFO, fmt.Sprint(v...))
}

func (l *Logger) Println(v ...interface{}) {
	l.Output(INFO, fmt.Sprintln(v...))
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if DEBUG < l.level {
		return
	}
	l.Output(DEBUG, fmt.Sprintf(format, v...))
}

func (l *Logger) Debug(v ...interface{}) {
	if DEBUG < l.level {
		return
	}
	l.Output(DEBUG, fmt.Sprintln(v...))
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if INFO < l.level {
		return
	}
	l.Output(INFO, fmt.Sprintf(format, v...))
}

func (l *Logger) Info(v ...interface{}) {
	if INFO < l.level {
		return
	}
	l.Output(INFO, fmt.Sprintln(v...))
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.Output(WARN, fmt.Sprintf(format, v...))
}

func (l *Logger) Warn(v ...interface{}) {
	l.Output(WARN, fmt.Sprintln(v...))
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.Output(ERROR, fmt.Sprintf(format, v...))
}

func (l *Logger) Error(v ...interface{}) {
	l.Output(ERROR, fmt.Sprintln(v...))
}

func (l *Logger) Fatal(v ...interface{}) {
	l.Output(FATAL, fmt.Sprint(v...))
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.Output(FATAL, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func (l *Logger) Fatalln(v ...interface{}) {
	l.Output(FATAL, fmt.Sprintln(v...))
	os.Exit(1)
}

func (l *Logger) Panic(v ...interface{}) {
	s := fmt.Sprint(v...)
	l.Output(PANIC, s)
	panic(s)
}

func (l *Logger) Panicf(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	l.Output(PANIC, s)
	panic(s)
}

func (l *Logger) Panicln(v ...interface{}) {
	s := fmt.Sprintln(v...)
	l.Output(PANIC, s)
	panic(s)
}

func (l *Logger) Stack(v ...interface{}) {
	s := fmt.Sprint(v...)
	s += "\n"
	buf := make([]byte, 1024*1024)
	n := runtime.Stack(buf, true)
	s += string(buf[:n])
	s += "\n"
	l.Output(ERROR, s)
}

func (l *Logger) Stat() (stats []int64) {
	l.mu.Lock()
	v := l.levelStats
	l.mu.Unlock()
	return v[:]
}

func isExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

func Print(v ...interface{}) {
	if std != nil {
		std.Output(INFO, fmt.Sprint(v...))
	}
	if std.isConsole {
		console.Output(INFO, fmt.Sprint(v...))
	}
}

func Printf(format string, v ...interface{}) {
	if std != nil {
		std.Output(INFO, fmt.Sprintf(format, v...))
	}
	if std.isConsole {
		console.Output(INFO, fmt.Sprintf(format, v...))
	}
}

func Println(v ...interface{}) {
	if std != nil {
		std.Output(INFO, fmt.Sprintln(v...))
	}
	if std.isConsole {
		console.Output(INFO, fmt.Sprintln(v...))
	}
}

func Debugf(format string, v ...interface{}) {
	if std == nil || DEBUG < std.level {
		return
	}

	std.Output(DEBUG, fmt.Sprintf(format, v...))

	if std.isConsole {
		console.Output(DEBUG, fmt.Sprintf(format, v...))
	}
}

func Debug(v ...interface{}) {
	if std == nil || DEBUG < std.level {
		return
	}

	std.Output(DEBUG, fmt.Sprintln(v...))

	if std.isConsole {
		console.Output(DEBUG, fmt.Sprintln(v...))
	}
}

func Infof(format string, v ...interface{}) {
	if std == nil || INFO < std.level {
		return
	}

	std.Output(INFO, fmt.Sprintf(format, v...))

	if std.isConsole {
		console.Output(INFO, fmt.Sprintf(format, v...))
	}
}

func Info(v ...interface{}) {
	if std == nil || INFO < std.level {
		return
	}

	std.Output(INFO, fmt.Sprintln(v...))

	if std.isConsole {
		console.Output(INFO, fmt.Sprintln(v...))
	}
}

func Warnf(format string, v ...interface{}) {
	if std == nil || WARN < std.level {
		return
	}

	std.Output(WARN, fmt.Sprintf(format, v...))

	if std.isConsole {
		console.Output(WARN, fmt.Sprintf(format, v...))
	}
}

func Warn(v ...interface{}) {
	if std == nil || WARN < std.level {
		return
	}

	std.Output(WARN, fmt.Sprintln(v...))

	if std.isConsole {
		console.Output(WARN, fmt.Sprintln(v...))
	}
}

func Errorf(format string, v ...interface{}) {
	if std == nil || ERROR < std.level {
		return
	}

	std.Output(ERROR, fmt.Sprintf(format, v...))

	if std.isConsole {
		console.Output(ERROR, fmt.Sprintf(format, v...))
	}
}

func Error(v ...interface{}) {
	if std == nil {
		return
	}

	std.Output(ERROR, fmt.Sprintln(v...))

	if std.isConsole {
		console.Output(ERROR, fmt.Sprintln(v...))
	}
}

func Fatal(v ...interface{}) {
	if std == nil {
		return
	}

	std.Output(FATAL, fmt.Sprint(v...))

	if std.isConsole {
		console.Output(FATAL, fmt.Sprint(v...))
	}
	os.Exit(1)
}

func Fatalf(format string, v ...interface{}) {
	if std == nil {
		return
	}

	std.Output(FATAL, fmt.Sprintf(format, v...))

	if std.isConsole {
		console.Output(FATAL, fmt.Sprintf(format, v...))
	}

	os.Exit(1)
}

func Fatalln(v ...interface{}) {
	if std == nil {
		return
	}

	std.Output(FATAL, fmt.Sprintln(v...))

	if std.isConsole {
		console.Output(FATAL, fmt.Sprintln(v...))
	}

	os.Exit(1)
}

func Panic(v ...interface{}) {
	if std == nil {
		return
	}

	s := fmt.Sprint(v...)

	std.Output(PANIC, s)

	if std.isConsole {
		console.Output(PANIC, s)
	}

	panic(s)
}

func Panicf(format string, v ...interface{}) {
	if std == nil {
		return
	}

	s := fmt.Sprintf(format, v...)

	std.Output(PANIC, s)

	if std.isConsole {
		console.Output(PANIC, s)
	}

	panic(s)
}

func Panicln(v ...interface{}) {
	if std == nil {
		return
	}

	s := fmt.Sprintln(v...)

	std.Output(PANIC, s)

	if std.isConsole {
		console.Output(PANIC, s)
	}

	panic(s)
}

func Stack(v ...interface{}) {
	if std == nil {
		return
	}

	s := fmt.Sprint(v...)
	s += "\n"
	buf := make([]byte, 1024*1024)
	n := runtime.Stack(buf, true)
	s += string(buf[:n])
	s += "\n"
	
	std.Output(ERROR, s)

	if std.isConsole {
		console.Output(ERROR, s)
	}
}

