#!/bin/sh
set -u -o pipefail
pip install -q --disable-pip-version-check --root-user-action=ignore httpx pyparsing pillow
python browser.py > out/results/browser.log 2>&1 &
browser=$!
if [ "$#" -eq 0 ]; then
  set -- \
    "oidcc-basic-certification-test-plan[server_metadata=discovery][client_registration=static_client]" out/config/static.json \
    "oidcc-config-certification-test-plan" out/config/static.json \
    "oidcc-formpost-basic-certification-test-plan[server_metadata=discovery][client_registration=static_client]" out/config/static.json \
    "oidcc-basic-certification-test-plan[server_metadata=discovery][client_registration=dynamic_client]" out/config/dynamic.json \
    "oidcc-rp-initiated-logout-certification-test-plan[response_type=code][client_registration=static_client]" out/config/static.json \
    "oidcc-backchannel-rp-initiated-logout-certification-test-plan[response_type=code][client_registration=static_client]" out/config/backchannel.json
fi
python out/suite/run-test-plan.py --export-dir out/results --verbose --expected-failures-file expected-failures.json --expected-skips-file expected-skips.json "$@" 2>&1 | tee out/results/summary.txt
status=$?
kill "$browser" 2>/dev/null
exit "$status"
