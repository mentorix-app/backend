#!/usr/bin/env bash
# merge_coverprofiles OUT IN... — merge Go text cover profiles written with
# -covermode=atomic (or count) into OUT by summing per-block hit counts.
#
# Profile format: a "mode: <mode>" header, then one line per block:
#   <file>:<start>.<col>,<end>.<col> <num_statements> <count>
# The same block may appear in several inputs (unit + integration runs of the
# same package); its counts add up, which is exactly what gocovmerge does for
# count-based modes. Kept in-repo so CI does not depend on `go run ...@latest`.
merge_coverprofiles() {
  local out="$1"
  shift
  awk '
    /^mode: / {
      if (mode == "") { mode = $2 }
      else if (mode != $2) { print "merge_coverprofiles: mixed modes " mode " vs " $2 > "/dev/stderr"; exit 1 }
      next
    }
    NF == 3 {
      key = $1 " " $2
      if (!(key in count)) { order[++n] = key }
      count[key] += $3
      next
    }
    NF > 0 { print "merge_coverprofiles: unexpected line: " $0 > "/dev/stderr"; exit 1 }
    END {
      if (mode == "") { mode = "atomic" }
      if (mode == "set") { print "merge_coverprofiles: mode set cannot be summed" > "/dev/stderr"; exit 1 }
      print "mode: " mode
      for (i = 1; i <= n; i++) { print order[i] " " count[order[i]] }
    }
  ' "$@" > "$out"
}
