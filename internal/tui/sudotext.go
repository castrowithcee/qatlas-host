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
	textSudoMoreCommands   kittui.TextID = "sudo.more"
	textAskChangelog       kittui.TextID = "sudo.ask.changelog"
	textWhatChangelog      kittui.TextID = "sudo.what.changelog"
	textAskKeyGenerate     kittui.TextID = "sudo.ask.keygenerate"
	textWhatKeyGenerate    kittui.TextID = "sudo.what.keygenerate"
	textAskKeySet          kittui.TextID = "sudo.ask.keyset"
	textWhatKeySet         kittui.TextID = "sudo.what.keyset"
)

func sudoEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textSudoErrCommand:     "qhost reported: %s",
		textSudoErrAborted:     "The run was interrupted before qhost answered. If the password prompt was cancelled, nothing was changed.",
		textSudoErrExit:        "The command ended with exit code %d and no answer.",
		textSudoErrOutput:      "The answer of qhost could not be read.",
		textSudoRemoteGitHub:   "This asks GitHub, so the request leaves the host.",
		textSudoRemoteRegistry: "This asks the registries of the images, so the requests leave the host.",
		textSudoMoreCommands:   "%d more commands of the same form follow, one after another.",
		textAskChangelog:       "Read the release notes before updating %s?",
		textWhatChangelog:      "Nothing is changed. The update itself is asked for separately afterwards.",
		textAskKeyGenerate:     "Generate the deploy key for %s?",
		textWhatKeyGenerate:    "A new Ed25519 key is created in the service. Its public part is shown afterwards and is entered in the git platform.",
		textAskKeySet:          "Store the private key as the deploy key for %s?",
		textWhatKeySet:         "The key is handed to the service on standard input and never shown. Its public part is shown afterwards and is entered in the git platform.",
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
		textSudoMoreCommands:   "%d weitere Befehle derselben Form folgen nacheinander.",
		textAskChangelog:       "Die Release Notes lesen, bevor %s aktualisiert wird?",
		textWhatChangelog:      "Es wird nichts geändert. Das Update selbst wird danach gesondert bestätigt.",
		textAskKeyGenerate:     "Den Deploy-Schlüssel für %s erzeugen?",
		textWhatKeyGenerate:    "Im Dienst entsteht ein neuer Ed25519-Schlüssel. Sein öffentlicher Teil wird danach angezeigt und in der Git-Plattform eingetragen.",
		textAskKeySet:          "Den privaten Schlüssel als Deploy-Schlüssel für %s hinterlegen?",
		textWhatKeySet:         "Der Schlüssel geht über die Standardeingabe an den Dienst und wird nie angezeigt. Sein öffentlicher Teil wird danach angezeigt und in der Git-Plattform eingetragen.",
	}
}
