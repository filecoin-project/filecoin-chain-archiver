package config

import (
	"bytes"
	"io"
	"net/url"
	"os"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"
)

func DefaultIndexServiceConfig() *IndexServiceConfig {
	return &IndexServiceConfig{}
}

func DefaultExportWorkerConfig() *ExportWorkerConfig {
	return &ExportWorkerConfig{}
}

type URL url.URL

// UnmarshalText implements interface for TOML decoding
func (u *URL) UnmarshalText(text []byte) error {
	d, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	*u = URL(*d)
	return err
}

func (u URL) MarshalText() ([]byte, error) {
	d := url.URL(u)
	return []byte(d.String()), nil
}

type Node struct {
	Address   string
	TokenPath string
}

type ExportWorkerConfig struct {
	Nodes []Node
}

type S3ResolverConfig struct {
	Endpoint      string
	Bucket        string
	AccessKeyPath string
	SecretKeyPath string
}

type IndexServiceConfig struct {
	S3Resolver S3ResolverConfig
}

func FromFile(path string, def interface{}) (interface{}, error) {
	file, err := os.Open(path)
	switch {
	case os.IsNotExist(err):
		return def, nil
	case err != nil:
		return nil, err
	}

	defer file.Close()
	return FromReader(file, def)
}

func FromReader(reader io.Reader, def interface{}) (interface{}, error) {
	cfg := def
	_, err := toml.DecodeReader(reader, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func ConfigComment(t interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("# Default config:\n")
	e := toml.NewEncoder(buf)
	if err := e.Encode(t); err != nil {
		return nil, xerrors.Errorf("encoding config: %w", err)
	}
	b := buf.Bytes()
	b = bytes.ReplaceAll(b, []byte("\n"), []byte("\n#"))
	b = bytes.ReplaceAll(b, []byte("#["), []byte("["))
	return b, nil
}
