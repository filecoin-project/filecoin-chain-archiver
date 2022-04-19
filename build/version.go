package build

var CurrentCommit string

const version = "v0.0.0-dev"

func Version() string {
	return version + CurrentCommit
}
