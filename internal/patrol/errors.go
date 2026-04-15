package patrol

import "fmt"

func errPatrolNotFound(name string) error {
	return fmt.Errorf("patrol %q not found", name)
}

func errPatrolNotRunnable(name string, state State) error {
	return fmt.Errorf("patrol %q is in state %s, cannot run", name, state)
}
