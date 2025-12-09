package classifier

// DynastyInfo contains information about a dynasty
type DynastyInfo struct {
	Name      string
	NameEn    string
	StartYear *int
	EndYear   *int
}

// GetDynastyInfo returns information about a dynasty by name
func GetDynastyInfo(name string) DynastyInfo {
	dynasties := map[string]DynastyInfo{
		"唐": {
			Name:      "唐",
			NameEn:    "Tang",
			StartYear: intPtr(618),
			EndYear:   intPtr(907),
		},
		"宋": {
			Name:      "宋",
			NameEn:    "Song",
			StartYear: intPtr(960),
			EndYear:   intPtr(1279),
		},
		"元": {
			Name:      "元",
			NameEn:    "Yuan",
			StartYear: intPtr(1271),
			EndYear:   intPtr(1368),
		},
		"五代": {
			Name:      "五代",
			NameEn:    "Five Dynasties",
			StartYear: intPtr(907),
			EndYear:   intPtr(960),
		},
		"先秦": {
			Name:      "先秦",
			NameEn:    "Pre-Qin",
			StartYear: intPtr(-2070),
			EndYear:   intPtr(-221),
		},
		"两汉": {
			Name:      "两汉",
			NameEn:    "Han",
			StartYear: intPtr(-206),
			EndYear:   intPtr(220),
		},
		"魏晋": {
			Name:      "魏晋",
			NameEn:    "Wei-Jin",
			StartYear: intPtr(220),
			EndYear:   intPtr(420),
		},
		"南北朝": {
			Name:      "南北朝",
			NameEn:    "Northern and Southern",
			StartYear: intPtr(420),
			EndYear:   intPtr(589),
		},
		"隋": {
			Name:      "隋",
			NameEn:    "Sui",
			StartYear: intPtr(581),
			EndYear:   intPtr(618),
		},
		"清": {
			Name:      "清",
			NameEn:    "Qing",
			StartYear: intPtr(1644),
			EndYear:   intPtr(1912),
		},
		"其他": {
			Name:   "其他",
			NameEn: "Other",
		},
	}

	if info, ok := dynasties[name]; ok {
		return info
	}

	return DynastyInfo{
		Name:   "其他",
		NameEn: "Other",
	}
}

func intPtr(i int) *int {
	return &i
}
