package streamlistpack

func getFieldsAndValues(data []string) (fields []string, values []string) {
	for i := 0; i < len(data); i += 2 {
		fields = append(fields, data[i])
		values = append(values, data[i+1])
	}
	return fields, values
}

func (s *StreamListpack) matchMasterFields(fields []string) bool {
	if len(fields) != len(s.Fields) {
		return false
	}

	for i := 0; i < len(fields); i++ {
		if fields[i] != s.Fields[i] {
			return false
		}
	}

	return true
}
