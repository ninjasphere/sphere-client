// +build !release

package main

import "github.com/bugsnag/bugsnag-go"

func init() {

	bugsnag.Configure(bugsnag.Configuration{
		APIKey:       "d6fc9adb2cc2880c0f644f8897402239",
		ReleaseStage: "development",
	})
}
