# onerep plan format

A plan is one JSON document describing a block of training weeks. The JSON Schema returned alongside this guide is the contract; this is how to use it well.

## Outline

```
plan: name, unit ("kg" or "lb", defaults to the user's unit), weeks (1-52), days[]
  day: name, only_weeks? (1-based week numbers), groups[]
    group: rest_s? (default 120), exercises[]
      exercise slot: slug, alternatives? (extra swap options), notes?, sets[]
        set line: kind?, count, reps | duration_s, load?, rpe?
```

Days are done in order each week. Exercise slugs must exist for the user: find them with `list_exercises`, and only create one with `create_exercise` when nothing fits.

## Values per week

Any number marked "per week" in the schema (`count`, `reps`, `duration_s`, `rest_s`, and the values inside `load` or `rpe`) may be a single value, used every week, or an array with exactly `weeks` entries, where entry i applies to week i+1. In a per-week `count` array, `null` or `0` leaves that set line out that week.

Use per-week arrays to program progression inside one document: `"count": [3, 3, 4, 2]` and `"load": {"pct_tm": [0.75, 0.8, 0.85, 0.65]}` in a 4-week block ramp up and then deload.

`only_weeks` restricts a whole day to some weeks, e.g. a test day in the last week.

## Set lines

- `count`: how many identical sets.
- `reps`: a number, a range string like `"6-10"`, or `"AMRAP"`. Timed exercises (planks, carries) use `duration_s` instead of `reps`.
- `kind`: `working` (default), `warmup`, `drop` or `amrap`. Warmups don't count toward PRs or muscle volume.
- `load`: how the weight is chosen, exactly one key:
  - `weight`: absolute weight in the plan's unit.
  - `pct_tm`: a fraction of the exercise's training max, e.g. 0.8. It needs a training max: check `get_exercise_stats` and set one with `set_training_max` if it is missing, or the lifter has to pick the weight.
  - `rpe`: a target RPE (6-10 in 0.5 steps). The weight comes from the lifter's recent estimated 1RM and the RTS table, so it only resolves for exercises with recent logged sets. The weight is re-estimated during the workout after each set.
  - `drop_pct`: a fraction below the previous set's actual weight, e.g. 0.2. It is not allowed on an exercise's first set line.
  - Leave `load` out for bodyweight, reps-only and timed work, or when the lifter should choose.
- `rpe` next to a `weight` or `pct_tm` load is only an informational target shown to the lifter.

Loads are rounded down to what the lifter's equipment can make (bar and plates, dumbbell pairs, cable stacks).

## Groups and supersets

A group with one exercise is a straight-set block: `rest_s` applies between its sets. A group with several exercises is a superset done A1, B1, A2, B2, …; `rest_s` applies after each full round, and an exercise with fewer sets drops out of the rotation when done.

`alternatives` lists extra exercises the lifter may swap to when equipment is busy; the catalog already knows common alternatives.

## Validation

`validate_plan` and `save_plan_draft` run the same checks the web editor does: the schema, then rules the schema can't express (slugs exist, per-week arrays have `weeks` entries, `only_weeks` in range, valid rep ranges, no `drop_pct` first, at least one day every week). Problems come back as `pointer: message`, where pointer is a JSON Pointer into the document (`/days/0/groups/1/exercises/0/sets/2/count`). Errors must be fixed; warnings (such as a `pct_tm` load without a training max) may be saved.

## Drafts, review and activation

`save_plan_draft` never changes what the lifter trains:

- no ids: a new plan with its first draft version
- `plan_id`: a new draft version of that plan (use this for the next block of a plan they follow)
- `version_id`: replaces that draft (only drafts; active and old versions never change)

Every save returns a `review_url`. The lifter opens it to compare the draft with the active version, week by week with resolved loads, and activates or discards it. Always give them that link, and a short note on what changed (also pass it as `note`).

## Example

```json
{
  "name": "Upper/Lower starter",
  "weeks": 4,
  "days": [
    {"name": "Upper", "groups": [
      {"rest_s": 180, "exercises": [
        {"slug": "barbell-bench-press", "alternatives": ["dumbbell-bench-press"], "sets": [
          {"kind": "warmup", "count": 2, "reps": 5, "load": {"pct_tm": 0.5}},
          {"count": [3, 3, 4, 2], "reps": 5, "load": {"pct_tm": [0.75, 0.8, 0.85, 0.65]}}
        ]}
      ]},
      {"rest_s": 90, "exercises": [
        {"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10", "load": {"rpe": 8}}]},
        {"slug": "dumbbell-lateral-raise", "sets": [{"count": 3, "reps": "12-15"}]}
      ]}
    ]},
    {"name": "Lower", "groups": [
      {"rest_s": 180, "exercises": [
        {"slug": "barbell-back-squat", "sets": [
          {"count": [3, 3, 4, 2], "reps": 5, "load": {"pct_tm": [0.75, 0.8, 0.85, 0.65]}},
          {"kind": "drop", "count": 1, "reps": 10, "load": {"drop_pct": 0.2}}
        ]}
      ]},
      {"rest_s": 60, "exercises": [
        {"slug": "plank", "sets": [{"count": 3, "duration_s": 45}]}
      ]}
    ]}
  ]
}
```
