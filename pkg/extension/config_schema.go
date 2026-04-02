package extension

// PasswordFieldsFromSchema extracts property names that have "format": "password"
// from a JSON Schema configSchema.
func PasswordFieldsFromSchema(schema map[string]any) []string {
	if len(schema) == 0 {
		return nil
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	var fields []string
	for name, v := range props {
		prop, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if fmt, ok := prop["format"].(string); ok && fmt == "password" {
			fields = append(fields, name)
		}
	}
	return fields
}
