# Triage Labels

The skills speak in terms of five canonical triage roles. This file maps those roles to the actual GitHub issue labels used in this repo.

| Label in mattpocock/skills | Label in our tracker | Meaning                                  |
| -------------------------- | -------------------- | ---------------------------------------- |
| `needs-triage`             | `needs-triage`       | Maintainer needs to evaluate this issue  |
| `needs-info`               | `needs-info`         | Waiting on reporter for more information |
| `ready-for-agent`          | `ready-for-agent`    | Fully specified, ready for an AFK agent  |
| `ready-for-human`          | `ready-for-human`    | Requires human implementation            |
| `wontfix`                  | `wontfix`            | Will not be actioned                     |

When a skill mentions a role (e.g. "apply the AFK-ready triage label"), use the corresponding label string from this table.

Create the labels in the GitHub repository once:

```bash
for label in needs-triage needs-info ready-for-agent ready-for-human wontfix; do
  gh label create "$label" --force
done
```

Edit the "Label in our tracker" column to match whatever vocabulary you actually use.
