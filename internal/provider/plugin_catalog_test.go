package provider

import "testing"

// A plugin model that says it only makes images or videos routes to the
// images or videos API, not chat; text among its outputs keeps it a chat
// model however many images it also gives (draw.go).
func TestPluginModelKinds(t *testing.T) {
	if !pluginDraws([]string{"image"}) || pluginDraws([]string{"image", "text"}) || pluginDraws(nil) || pluginDraws([]string{"text"}) {
		t.Error("pluginDraws: only image-alone output draws")
	}
	if !pluginFilms([]string{"video"}) || pluginFilms([]string{"video", "text"}) || pluginFilms([]string{"image"}) || pluginFilms(nil) {
		t.Error("pluginFilms: only video-alone output films")
	}
}
