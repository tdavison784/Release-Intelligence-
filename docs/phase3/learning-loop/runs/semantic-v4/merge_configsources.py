"""Append researched configSources blocks to products/<id>.yaml (semantic-4, L1).

    python3 merge_configsources.py <research-dir> <products-dir> <id>...

Each research file holds {product, configSources}; the block is appended to the definition under a
provenance comment (upstream-documented; every reference URL was fetched and every quote checked
verbatim by the researcher). A definition that already has configSources is left unchanged.
"""
import sys, yaml, os

research, products = sys.argv[1], sys.argv[2]
for pid in sys.argv[3:]:
    doc = yaml.safe_load(open(os.path.join(research, pid + ".yaml")))
    assert doc["product"] == pid, (pid, doc["product"])
    path = os.path.join(products, pid + ".yaml")
    text = open(path).read()
    if "\nconfigSources:" in text:
        print(f"{pid}: already has configSources, skipped")
        continue
    block = yaml.safe_dump({"configSources": doc["configSources"]}, sort_keys=False, width=110, allow_unicode=True)
    header = ("\n# Where this product reads its configuration, from its upstream documentation (semantic-4,\n"
              "# LOOP-DIAGNOSIS-2 L1): every reference was fetched and its quote checked verbatim. Used by the\n"
              "# learning loop's prompts only (docs/ARCHITECTURE.md, definition constructs); never from eval data.\n")
    with open(path, "a") as fh:
        fh.write(("" if text.endswith("\n") else "\n") + header + block)
    print(f"{pid}: {len(doc['configSources'])} config sources")
