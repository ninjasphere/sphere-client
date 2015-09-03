#!/bin/bash

VERSION=client-helper.sh-v1.0.0
export SPHERE_CLIENT_COMMANDS=/opt/ninjablocks/sphere-client/commands
export SPHERE_CLIENT_SITE_PREFERENCE_HELPERS=/opt/ninjablocks/sphere-client/site-preference-helpers

# die, print an error message and exit the current shell
die() {
	echo "$*" 1>&2
 	exit 1
}

client-args() {
	(
	    test -r /etc/profile && . /etc/profile
	    test -r /etc/ninja-hardware && . /etc/ninja-hardware
	    test -r /etc/ninja-release && . /etc/ninja-release
	    test -r /etc/default/ninja && . /etc/default/ninja

	    if test -z "$NINJA_SPHERE_CLIENT_ARGS"; then
		    TARGET_BRANCH=`echo $NINJA_OS_BUILD_TARGET | cut -d'-' -f2`
		    case "$TARGET_BRANCH" in
		    	stable|testing)
			    	NINJA_SPHERE_CLIENT_ARGS="--cloud-production"
				;;
		    esac
		fi
		test -n "$NINJA_SPHERE_CLIENT_ARGS" && echo $NINJA_SPHERE_CLIENT_ARGS
	)
}

# parse the command arguments and invoke handling functions as appropriate
main() {
	local cmd=$1
	shift 1
	case "$cmd" in
	client-args)
		client-args
	;;
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