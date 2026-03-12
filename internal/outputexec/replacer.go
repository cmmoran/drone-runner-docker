package outputexec

import (
	"io"
	"strings"

	runtime "github.com/drone/runner-go/pipeline/runtime"
)

type replacer struct {
	w io.WriteCloser
	r *strings.Replacer
}

func newReplacer(w io.WriteCloser, secrets []runtime.Secret) io.WriteCloser {
	var oldnew []string
	for _, secret := range secrets {
		v := secret.GetValue()
		if len(v) == 0 || !secret.IsMasked() {
			continue
		}
		for _, part := range strings.Split(v, "\n") {
			part = strings.TrimSpace(part)
			if len(part) < 2 {
				continue
			}
			oldnew = append(oldnew, part, "******")
		}
	}
	if len(oldnew) == 0 {
		return w
	}
	return &replacer{
		w: w,
		r: strings.NewReplacer(oldnew...),
	}
}

func (r *replacer) Write(p []byte) (int, error) {
	_, err := r.w.Write([]byte(r.r.Replace(string(p))))
	return len(p), err
}

func (r *replacer) Close() error {
	return r.w.Close()
}
