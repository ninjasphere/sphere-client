#!/bin/bash

VERSION=client-helper.sh-v1.0.0
export SPHERE_CLIENT_COMMANDS=/opt/ninjablocks/sphere-client/commands
export SPHERE_CLIENT_SITE_PREFERENCE_HELPERS=/opt/ninjablocks/sphere-client/site-preference-helpers

# die, print an error message and exit the current shell
die() {
	echo "$*" 1>&2
 	exit 1
}

# parse the command arguments and invoke handling functions as appropriate
main() {
	local cmd=$1
	shift 1
	case "$cmd" in
	apply-site-preferences)
		rc=0
		test -d "$SPHERE_CLIENT_SITE_PREFERENCE_HELPERS" && cd "$SPHERE_CLIENT_SITE_PREFERENCE_HELPERS" && find . -type f -maxdepth 1 -exec basename {} \; | sort | while read c; do
			if test -x "${SPHERE_CLIENT_SITE_PREFERENCE_HELPERS}/$c"; then
				"${SPHERE_CLIENT_SITE_PREFERENCE_HELPERS}/$c" "$@" || rc=1
			fi
			test $rc -eq 0
		done
	;;
	version)
		echo "$VERSION"
	;;
	*)
		die "usage: '$cmd' is not supported by this version of client-helper.sh"
	;;
	esac
}

main "$@"