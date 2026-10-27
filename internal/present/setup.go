package present

import "github.com/itsgiahung/502Drive/internal/i18n"

func SetupStep(name, lang string) string {
	key := map[string]i18n.Key{"daemon": i18n.SetupDaemon, "database": i18n.SetupDaemon, "oauth_client": i18n.SetupOAuth, "account": i18n.SetupAccount, "destination": i18n.SetupDestination}[name]
	if key == "" {
		return name
	}
	return i18n.T(lang, key)
}
