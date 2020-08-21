package repl

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/tarm/serial"
)

// Repl manages the serial port REPL connection.
type Repl struct {
	Port *serial.Port
}

// Connect opens a connection to the serial port and returns Repl instance.
func Connect(device string, baud int) (*Repl, error) {
	c := &serial.Config{
		Name:        device,
		Baud:        baud,
		ReadTimeout: time.Millisecond * 500,
	}
	p, err := serial.OpenPort(c)
	if err != nil {
		return nil, err
	}
	// send ctrl-C twice to stop any running code
	_, err = p.Write([]byte("\r\x03\x03"))
	if err != nil {
		return nil, err
	}
	r := &Repl{
		Port: p,
	}
	return r, nil
}

// ReadUntil reads from the Repl until the ending byte string is found. If w is
// supplied it'll Write data there instead of accumulating it.
func (r *Repl) ReadUntil(ending []byte, w io.Writer) ([]byte, error) {
	b := make([]byte, 1)
	data := make([]byte, 0, 1024)
	for {
		_, err := r.Port.Read(b)
		if err != nil {
			return nil, err
		}
		if w == nil {
			data = append(data, b[0])
		} else {
			if b[0] != 0x04 {
				_, err = w.Write(b)
				if err != nil {
					return nil, err
				}
			}
			data = b
		}
		if bytes.HasSuffix(data, ending) {
			return data, nil
		}
	}
}

// EnterRawMode will send ctrl-A to Repl to enter raw terminal mode.
func (r *Repl) EnterRawMode() error {
	// ctrl-A: enter raw REPL
	_, err := r.Port.Write([]byte("\r\x01"))
	if err != nil {
		return err
	}
	_, err = r.ReadUntil([]byte("raw REPL; CTRL-B to exit\r\n"), nil)
	return err
}

// ExitRawMode will send ctrl-B to Repl to return to normal terminal mode.
func (r *Repl) ExitRawMode() error {
	// ctrl-B: enter friendly REPL
	_, err := r.Port.Write([]byte("\r\x02"))
	return err
}

// SoftReboot will send ctrl-D to Repl to perform a soft reboot.
func (r *Repl) SoftReboot() error {
	_, err := r.Port.Write([]byte("\x04"))
	if err != nil {
		return err
	}
	_, err = r.ReadUntil([]byte("soft reboot\r\n"), nil)
	if err != nil {
		return err
	}
	_, err = r.ReadUntil([]byte("raw REPL; CTRL-B to exit\r\n"), nil)
	return err
}

// ExecRaw will execute code without following the results.
func (r *Repl) ExecRaw(code []byte) error {
	_, err := r.ReadUntil([]byte(">"), nil)
	if err != nil {
		return err
	}
	_, err = r.Port.Write(code)
	if err != nil {
		return err
	}
	_, err = r.Port.Write([]byte("\x04"))
	if err != nil {
		return err
	}
	resp := make([]byte, 2)
	_, err = io.ReadAtLeast(r.Port, resp, 2)
	if err != nil {
		return err
	}
	if !bytes.Equal(resp, []byte("OK")) {
		return errors.New("could not exec command")
	}
	return nil
}

// Follow will read the response data and/or error from executing code.
func (r *Repl) Follow(w io.Writer) ([]byte, []byte, error) {
	data, err := r.ReadUntil([]byte("\x04"), w)
	if err != nil {
		return nil, nil, err
	}
	if len(data) > 0 {
		data = data[:len(data)-1]
	}
	dataErr, err := r.ReadUntil([]byte("\x04"), nil)
	if err != nil {
		return nil, nil, err
	}
	if len(dataErr) > 0 {
		dataErr = dataErr[:len(dataErr)-1]
	}
	return data, dataErr, nil
}

// Exec will execute code and read the response or error. If w is supplied it
// will call Write to pass the data instead of accumulating it.
func (r *Repl) Exec(code []byte, w io.Writer) ([]byte, error) {
	err := r.ExecRaw(code)
	if err != nil {
		return nil, err
	}
	data, dataErr, err := r.Follow(w)
	if err != nil {
		return nil, err
	}
	if len(dataErr) > 0 {
		return nil, errors.New(string(dataErr))
	}
	return data, nil
}

// Cat reads the contents of a file
func (r *Repl) Cat(w io.Writer, f string) error {
	code := []byte(`with open("` + f + `") as f:
	while True:
		b = f.read(256)
		if not b:
			break
		print(b, end='')`)
	_, err := r.Exec(code, w)
	if err != nil {
		return err
	}
	return nil
}

// Cd changes the current working directory
func (r *Repl) Cd(d string) error {
	code := []byte("import uos\nuos.chdir(\"" + d + "\")")
	_, err := r.Exec(code, nil)
	if err != nil {
		return err
	}
	return nil
}

// Get copies a file from the MicroPython device to the local machine
func (r *Repl) Get(dst, src string) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	_, err = r.Exec([]byte(`from ubinascii import b2a_base64
f=open("`+src+`",'rb')
`), nil)
	if err != nil {
		return err
	}
	for {
		var b bytes.Buffer
		_, err = r.Exec([]byte(`d=str(b2a_base64(f.read(256)),'ascii')
print(d.strip(),end='')
`), &b)
		if err != nil {
			return err
		}
		x, err := base64.StdEncoding.DecodeString(string(b.Bytes()))
		if err != nil {
			return err
		}
		if len(x) == 0 {
			break
		}
		f.Write(x)
	}
	return nil
}

// Ls lists the contents of the current directory
func (r *Repl) Ls() ([]string, error) {
	code := []byte(`import uos
for f in uos.ilistdir('.'):
	print(f[0], end='/ ' if f[1] & 0x4000 else ' ')
`)
	b := &strings.Builder{}
	_, err := r.Exec(code, b)
	if err != nil {
		return nil, err
	}
	s := strings.TrimRight(b.String(), " ")
	return strings.Split(s, " "), nil
}

// Mkdir makes a new directory
func (r *Repl) Mkdir(d string) error {
	code := []byte("import uos\nuos.mkdir('" + d + "')")
	_, err := r.Exec(code, nil)
	if err != nil {
		return err
	}
	return nil
}

// Put copies a file from the local machine to the MicroPython device
func (r *Repl) Put(dst, src string) error {
	f, err := os.OpenFile(src, os.O_RDONLY, 0666)
	if err != nil {
		return err
	}
	_, err = r.Exec([]byte(`from ubinascii import a2b_base64
f=open("`+dst+`",'wb')
w=lambda x:f.write(a2b_base64(x))
`), nil)
	if err != nil {
		return err
	}
	b := make([]byte, 256)
	for {
		n, err := f.Read(b)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		e := base64.StdEncoding.EncodeToString(b[:n])
		_, err = r.Exec([]byte("w(\""+e+"\")\n"), nil)
		if err != nil {
			return err
		}
	}
	_, err = r.Exec([]byte("f.close()"), nil)
	if err != nil {
		return err
	}
	return nil
}

// Cwd returns the current working directory
func (r *Repl) Cwd() (string, error) {
	code := []byte("import uos\nprint(uos.getcwd(),end='')")
	b := &strings.Builder{}
	_, err := r.Exec(code, b)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// Rm removes a file
func (r *Repl) Rm(f string) error {
	code := []byte("import uos\nuos.remove(\"" + f + "\")")
	_, err := r.Exec(code, nil)
	if err != nil {
		return err
	}
	return nil
}

// Rmdir removes a directory
func (r *Repl) Rmdir(d string) error {
	code := []byte("import uos\nuos.rmdir('" + d + "')")
	_, err := r.Exec(code, nil)
	if err != nil {
		return err
	}
	return nil
}

// Upload all files from the local directory to the MicroPython device
func (r *Repl) Upload() error {
	fs, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	for _, f := range fs {
		if f.IsDir() {
			continue
		}
		fn := f.Name()
		fmt.Println("Uploading", fn, "...")
		err = r.Put(fn, fn)
		if err != nil {
			return err
		}
	}
	return nil
}
