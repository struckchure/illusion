module {{ .Scaffold.module }}

go 1.25.3
{{- if .Scaffold.illusion_path }}

// Build against a local checkout of illusion.
replace github.com/struckchure/illusion => {{ .Scaffold.illusion_path }}
{{- end }}
