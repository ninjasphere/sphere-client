#!/bin/bash

VERSION=client-helper.sh-v1.0.0
export SPHERE_CLIENT_COMMANDS=/opt/ninjablocks/sphere-client/commands
export SPHERE_CLIENT_SITE_PREFERENCE_HELPERS=/opt/ninjablocks/sphere-client/site-preference-helpers

# die, print an error message and exit the current shell
die() {
	echo "$*" 1>&2
 	exit 1
}

# wrap the execution of a command as a json object.
raw_exec() {
	("$@") > /tmp/$$.out 2>/tmp/$$.err
	status=$?
	trap "rm /tmp/$$.out /tmp/$$.err" EXIT
	jq -c . <<EOF
{
	"output": $(cat /tmp/$$.out | jq -R . | jq -s -c .),
	"error": $(cat /tmp/$$.err | jq -R . | jq -s -c .),
	"status": $status
}
EOF
	exit $status
}

# execute a command that produces well-formed json output.
json_exec() {
	("$@") > /tmp/$$.out 2>/tmp/$$.err
	status=$?
	trap "rm /tmp/$$.out /tmp/$$.err" EXIT
	if jq . < /tmp/$$.out > /dev/null; then # check that we actually have json
		jq -c . <<EOF
{
	"data": $(cat /tmp/$$.out),
	"error": $(cat /tmp/$$.err | jq -R . | jq -s -c .),
	"status": $status
}
EOF
	else
		# we don't have json, so capture the output
		status=$?
		jq -c . <<EOF
{
	"output": $(cat /tmp/$$.out | jq -R . | jq -s -c .),
	"error": $(cat /tmp/$$.err | jq -R . | jq -s -c .),
	"status": $status
}
EOF
	fi
	exit $status
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
	exec)
		cmd=$1
		shift 1

		if test -x "${SPHERE_CLIENT_COMMANDS}/json/$cmd"; then
			json_exec "${SPHERE_CLIENT_COMMANDS}/json/$cmd" "$@"
		elif test -x "${SPHERE_CLIENT_COMMANDS}/raw/$cmd"; then
			raw_exec "${SPHERE_CLIENT_COMMANDS}/raw/$cmd" "$@"
		else
			raw_exec die "'$cmd' is not supported command."
		fi
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