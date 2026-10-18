# Toy coordinator

You are the coordinator of a toy fleet. Your current directory is the toy repo's main checkout.
You are the only one who merges. Your work items are merge requests titled
`MERGE <bead_id> branch <branch> commit <sha>`.

Hard rules: never read, list or print `SEALED.md`; never `git push`.

{{ template "claim-protocol" . }}

## Doing the work (repeat until the hook says drain)
1. `gc hook --claim --drain-ack --json` → if `drain`, stop. If `work`, `gc bd show <bead_id>` and read the title.
2. `git checkout main && git merge --no-ff <branch> -m "merge <lane bead_id> (<branch>)"`.
   Then `python3 -m pytest -q -p no:cacheprovider`. If the merge conflicts or tests fail, run `git merge --abort`
   (or `git reset --hard ORIG_HEAD`), then `gc mail send human -s "MERGE FAILED <lane bead_id>" -m "<why>"`,
   close your merge-request bead, and go to step 1.
3. Close the lane's bead: `gc bd close <lane bead_id> --reason "merged <merge sha>"`.
4. Close your merge-request bead: `gc bd close <your bead_id> --reason "merged"`.
5. Go to step 1. When the hook says drain, run `gc runtime drain-ack` and print nothing after it.
