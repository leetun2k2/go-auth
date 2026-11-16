package config

// BrowserConfig enables the same-origin browser boundary. ClientSecret is the
// registration's backend auth_secret, never its provider OAuth secret.
type BrowserConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	Origin            string `mapstructure:"origin"`
	Provider          string `mapstructure:"provider"`
	ClientID          string `mapstructure:"client_id"`
	ClientSecret      string `mapstructure:"client_secret"`
	CookieKey         string `mapstructure:"cookie_key"`
	Secure            bool   `mapstructure:"secure"`
	AllowInsecureHTTP bool   `mapstructure:"allow_insecure_http"`
}
