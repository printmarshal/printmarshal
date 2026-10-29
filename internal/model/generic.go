package model

import "strings"

// genericModelPrefixes start the names a network card or print server reports
// in place of the printer's model: an HP Jetdirect's sysDescr begins
// "HP ETHERNET MULTI-ENVIRONMENT,ROM none,JETDIRECT,...".
var genericModelPrefixes = []string{"HP ETHERNET", "HP JETDIRECT", "JETDIRECT"}

// IsGenericModel reports whether s names a print server or network card
// rather than a printer model. A device only reports one when the read that
// carries its real model (HP's PML model name, IPP printer-make-and-model)
// failed or has not run, so a generic name is a last resort: it may fill an
// empty model, and must never replace a specific one.
func IsGenericModel(s string) bool {
	u := strings.ToUpper(strings.TrimSpace(s))
	for _, p := range genericModelPrefixes {
		if strings.HasPrefix(u, p) {
			return true
		}
	}
	return false
}
