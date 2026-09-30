package compatibility

import "errors"

func makeFIFO(string) error {
	return errors.New("POSIX named pipes do not exist in the Windows filesystem")
}
