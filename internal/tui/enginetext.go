package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the engines: the headings of the sections of the Stacks area and the choice of the deploy service in
// Settings.
const (
	textEngineHeading    kittui.TextID = "engine.heading"
	textKeyInstance      kittui.TextID = "key.instance"
	textInstanceLine     kittui.TextID = "instance.line"
	textInstanceChoose   kittui.TextID = "instance.choose"
	textInstanceChooseIn kittui.TextID = "instance.choosein"
)

func engineEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textEngineHeading:    "Engine %s",
		textKeyInstance:      "instance",
		textInstanceLine:     "Instance: %s",
		textInstanceChoose:   "Choose the instance first.",
		textInstanceChooseIn: "Several deploy services exist and none is the default. {instance} chooses the instance whose settings this page shows.",
	}
}

func engineGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textEngineHeading:    "Engine %s",
		textKeyInstance:      "Instanz",
		textInstanceLine:     "Instanz: %s",
		textInstanceChoose:   "Wähle zuerst die Instanz.",
		textInstanceChooseIn: "Es gibt mehrere Deploy-Dienste, und keiner ist der Standard. {instance} wählt die Instanz, deren Einstellungen diese Seite zeigt.",
	}
}
