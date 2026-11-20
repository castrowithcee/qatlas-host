package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the changes an interface without root makes through sudo. What qhost reports comes in as an
// argument; sudo, root, qhost, GitHub and the registries stay as they are in every language.
const (
	textSudoErrCommand     kittui.TextID = "sudo.err.command"
	textSudoErrAborted     kittui.TextID = "sudo.err.aborted"
	textSudoErrExit        kittui.TextID = "sudo.err.exit"
	textSudoErrOutput      kittui.TextID = "sudo.err.output"
	textSudoRemoteGitHub   kittui.TextID = "sudo.remote.github"
	textSudoRemoteRegistry kittui.TextID = "sudo.remote.registry"
)

func sudoEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textSudoErrCommand:     "qhost reported: %s",
		textSudoErrAborted:     "The run was interrupted before qhost answered. If the password prompt was cancelled, nothing was changed.",
		textSudoErrExit:        "The command ended with exit code %d and no answer.",
		textSudoErrOutput:      "The answer of qhost could not be read.",
		textSudoRemoteGitHub:   "This asks GitHub, so the request leaves the host.",
		textSudoRemoteRegistry: "This asks the registries of the images, so the requests leave the host.",
	}
}

func sudoGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textSudoErrCommand:     "qhost meldet: %s",
		textSudoErrAborted:     "Der Lauf wurde unterbrochen, bevor qhost antwortete. Wurde die Passwortabfrage abgebrochen, wurde nichts geändert.",
		textSudoErrExit:        "Der Befehl endete mit Exit-Code %d und ohne Antwort.",
		textSudoErrOutput:      "Die Antwort von qhost ließ sich nicht lesen.",
		textSudoRemoteGitHub:   "Dafür wird GitHub angefragt, die Anfrage verlässt also den Host.",
		textSudoRemoteRegistry: "Dafür werden die Registries der Images angefragt, die Anfragen verlassen also den Host.",
	}
}
