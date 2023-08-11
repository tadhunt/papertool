#!/bin/bash

set -o nounset
set -o errexit

config_verbose="false"

function fatal {
	echo FAILURE: $* 1>&2
	exit 1
}

function warn {
	echo WARNING: $* 1>&2
}

function info {
	echo INFO: $* 1>&2
}

config_norun="false"
function run {
	if [ "${config_norun}" = "true" ] ; then
		echo "NORUN: $*" 1>&2
		return 0
	fi

	echo "RUN: $@" 1>&2
	if ! "$@" ; then
		fatal "$@"
	fi
}

function repo_has_changes {
	local result="$(git status --porcelain)"
	if [ -z "${result}" ] ; then
		return 1
	fi

	return 0
}

function repo_last_commit {
	git rev-parse HEAD
}

function semver_update {
	semver="${semver_major}.${semver_minor}.${semver_patch}"

	if [ ! -z "${semver_prere}" ] ; then
		semver="${semver}-${semver_prere}"
	fi

	if [ ! -z "${semver_build}" ] ; then
		semver="${semver}+${semver_build}"
	fi

	if [ "${config_verbose}" = "true" ] ; then
		info "major : ${semver_major}"
		info "minor : ${semver_minor}"
		info "patch : ${semver_patch}"
		info "prere : ${semver_prere}"
		info "build : ${semver_build}"
		info "semver: ${semver}"
	fi
}

function semver_parse {
	local vers="${1}"

	#
	# Parse the tag 
	#
	local semver_regex='^([0-9]+)\.([0-9]+)\.([0-9]+)(.*)'

	if ! [[ "${vers}" =~ ${semver_regex} ]] ; then
		warn "syntax error: ${vers}: not a semantic version"
		return 1
	fi

	local sep=""
	local extra=""

	semver_major="${BASH_REMATCH[1]}"
	semver_minor="${BASH_REMATCH[2]}"
	semver_patch="${BASH_REMATCH[3]}"
	semver_prere=""
	semver_build=""

	local extra="${BASH_REMATCH[4]}"

	if [ ! -z "${extra}" ] ; then
		local prere_regex='^-([^+]+)$'
		local build_regex='^\+(.*)'
		local pandb_regex='^-([^+]+)\+(.*)$'

		if [[ "${extra}" =~ ${prere_regex} ]] ; then
			semver_prere="${BASH_REMATCH[1]}"
		elif [[ "${extra}" =~ ${build_regex} ]] ; then
			semver_build="${BASH_REMATCH[1]}"
		elif [[ "${extra}" =~ ${pandb_regex} ]] ; then
			semver_prere="${BASH_REMATCH[1]}"
			semver_build="${BASH_REMATCH[2]}"
		else
			warn "syntax error: ${vers}: illegal pre-release or build"
			return 1
		fi
	fi

	semver_update
}


function cinfo_fetch {
	cinfo_commit="$(git rev-parse HEAD)"

	local tag="$(git describe --contains "${cinfo_commit}" 2>/dev/null || true)"

	cinfo_tag="${tag}"
}

function cinfo_get_vers {
	local vers="$(git describe --abbrev=0 --tags 2>/dev/null)"

	echo "${vers}"
}

#
# Manages Semantic Version (https://semver.org) Tags in the repository
#
function repo_tag {
	if [ $# -lt 1 ] ; then
		fatal "missing args"
	fi

	run cinfo_fetch

	#
	# Fail if the current version is already tagged
	#
	if [ ! -z "${cinfo_tag}" ] ; then
		warn "Last commit (${cinfo_commit}) is already tagged as: ${cinfo_tag}"
		return 1
	fi

	#
	# Find or create the last tag
	#
	local vers="$(cinfo_get_vers)"

	if [ -z "${vers}" ] ; then
		warn "Creating initial tag"
		vers="0.0.0"
	fi

	run semver_parse "${vers}"

	#
	# Update the tag
	#
	while [ $# -gt 0 ] ; do
		local mode="${1}"
		shift

		case "${mode}" in
		increment-major)
			semver_major="$((semver_major+1))"
			;;
		increment-minor)
			semver_minor="$((semver_minor+1))"
			;;
		increment-patch)
			semver_patch="$((semver_patch+1))"
			;;
		set-prerelease)
			semver_prere="${1}"
			shift
			;;
		set-build)
			semver_build="${1}"
			shift
			;;
		*)
			fatal "$0: unhandled mode: ${mode}"
		esac
	done

	run semver_update

	#
	# Tag the latest version with the new tag
	#
	run git tag "${semver}" "${cinfo_commit}"
	run git push --tags

	info "Tagged ${cinfo_commit} with ${semver}"

	return 0
}

function golang_mkversion {
	local vers="${1}"
	local package="${2}"
	local file="${3}"

	local goconst="$(echo ${package} | awk '{print toupper(substr($0, 0, 1))tolower(substr($0, 2))}')"

	echo "package ${package}"				 > "${file}"
	echo ""							>> "${file}"
	echo "const ${goconst}SemanticVersion = \"${vers}\""	>> "${file}"
}

function html_mkversion {
	local vers="${1}"
	local tool="${2}"
	local file="${3}"

	echo "<html>"				 > "${file}"
	echo " <body>"				>> "${file}"
	echo "  <h1>${tool} Version</h1>"	>> "${file}"
	echo "  <p>${vers}</p>"			>> "${file}"
	echo " </body>"				>> "${file}"
	echo "</html>"				>> "${file}"
}

function main {
	if [ $# -lt 1 ] ; then
		fatal "missing args"
	fi

	local cmd="${1}"
	shift

	local now="$(date "+%Y-%m-%d-%H-%M-%S")"

	case "${cmd}" in
	repo-tag-*)
		if repo_has_changes ; then
			fatal "repo has uncommitted changes"
		fi

		local tagcmd="${cmd/repo-tag-/}"

		run repo_tag "$@" increment-${tagcmd} set-build "${now}" || fatal "Failed to tag version"
		;;
	version-make-*)
		test $# -lt 2 && fatal "$0: ${cmd}: bad args: expected: ${cmd} tool filename [...]"

		local type="${cmd/version-make-/}"
		local tool="${1}"
		shift

		run cinfo_fetch
		local vers="$(cinfo_get_vers)"

		if [ -z "${vers}" ] ; then
			fatal "$0: ${cmd}: no version to use"
		fi

		run semver_parse "${vers}"

		while [ $# -gt 0 ] ; do
			local filename="${1}"
			shift

			case "${type}" in
			golang)
				run golang_mkversion "${semver}" "${tool}" "${filename}"
				;;
			html)
				run html_mkversion "${semver}" "${tool}" "${filename}"
				;;
			*)
				fatal "${cmd}: Unknown subcmd: ${type}"
				;;
			esac
		done
		;;
	*)
		fatal "Unknown cmd: ${cmd}"
	esac
}

main "$@"
