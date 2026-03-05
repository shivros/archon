package app

import "control/internal/types"

func fromAppStateSplit(pref *types.AppStateSplitPreference) *SplitPreference {
	if pref == nil {
		return nil
	}
	return &SplitPreference{Columns: pref.Columns, Ratio: pref.Ratio}
}

func toAppStateSplit(pref *SplitPreference) *types.AppStateSplitPreference {
	if pref == nil {
		return nil
	}
	return &types.AppStateSplitPreference{Columns: pref.Columns, Ratio: pref.Ratio}
}
