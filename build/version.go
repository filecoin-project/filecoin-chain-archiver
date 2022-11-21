package build

var CurrentCommit string

const version = "v1.1.1"

func Version() string {
	return version + CurrentCommit
}
