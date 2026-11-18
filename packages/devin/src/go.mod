module github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin

go 1.26.8

require github.com/yetone/magpie v0.0.0

require golang.org/x/sys v0.49.0 // indirect

// Local development only; release consumers pin a published fork revision.
replace github.com/yetone/magpie => ../../../../magpie
