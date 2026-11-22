package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of choosing the Docker engine of a folder to take over, and of showing it with a proposal.
const (
	textFieldEngine     kittui.TextID = "folderengine.field"
	textHintEngine      kittui.TextID = "folderengine.hint"
	textHintEngineFound kittui.TextID = "folderengine.hint.found"
	textEngineRuns      kittui.TextID = "folderengine.runs"
	textDetailEngine    kittui.TextID = "folderengine.detail"
	textEngineDiffers   kittui.TextID = "folderengine.differs"
	textWhatEngine      kittui.TextID = "folderengine.what"
	textPropFieldEngine kittui.TextID = "folderengine.prop.field"
)

func folderEngineEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textFieldEngine:     "Engine",
		textHintEngine:      "the Docker engine whose deploy service runs the stacks of the folder; another engine than system inspects the folder again",
		textHintEngineFound: "preselected is the engine where containers of the folder run; the folder is inspected again for another one",
		textEngineRuns:      "%s (%d containers of the folder run here)",
		textDetailEngine:    "engine: %s",
		textEngineDiffers: "Containers of the folder run in the engine %s, not in %s. Deploying would start a second set in %s and leave " +
			"the others running; nothing moves them.",
		textWhatEngine:      "The folder belongs to the engine %s.",
		textPropFieldEngine: "Engine",
	}
}

func folderEngineGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textFieldEngine:     "Engine",
		textHintEngine:      "die Docker-Engine, deren Deploy-Dienst die Stacks des Ordners betreibt; eine andere als system prüft den Ordner neu",
		textHintEngineFound: "vorgewählt ist die Engine, in der Container des Ordners laufen; für eine andere wird der Ordner neu geprüft",
		textEngineRuns:      "%s (hier laufen %d Container des Ordners)",
		textDetailEngine:    "Engine: %s",
		textEngineDiffers: "Container des Ordners laufen in der Engine %s, nicht in %s. Ein Deploy würde in %s eine zweite Gruppe starten " +
			"und die anderen weiterlaufen lassen; nichts verschiebt sie.",
		textWhatEngine:      "Der Ordner gehört zur Engine %s.",
		textPropFieldEngine: "Engine",
	}
}
