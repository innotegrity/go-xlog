package handlers

import "fmt"

// try implements try/catch-like functionality to try a function and recover from any errors or panics that may occur.
func try(callback func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("unexpected error: %+v", r)
			}
		}
	}()

	err = callback()
	return
}
