// sshc runs ssh, scp, sftp, rsync and ssh-copy-id and answers their prompt for
// a login password or an SSH key passphrase from a stored secret.
//
// The program itself lives in internal/sshc; this file is only the entry
// point, kept at the module root so that "go install <module>@latest" works.
package main

import (
	"os"

	"github.com/W-Industries-Luke/sshc/internal/sshc"
)

func main() {
	os.Exit(sshc.Main(os.Args[1:]))
}
