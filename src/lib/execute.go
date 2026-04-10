package lib

import (
	"fmt"
	"os/exec"
)

func ExecuteCommand(arg string) error {
	command := exec.Command("bash", "-c", arg)
	output, err := command.CombinedOutput()
	fmt.Println(string(output))
	return err
}
