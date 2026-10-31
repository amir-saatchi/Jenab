# L4_refresh_short

Refresh memory on a 6-turn chat

Good behaviour: In 6 turns the user states the project's goal, the team's rule of no chemical pesticides, a preference for dates as YYYY-MM-DD, and a harvest report on Fridays, which a later turn moves to Mondays; one turn gives a harvest count. Refresh memory keeps the rule and the newer report day. Whether the old day, the count or the date format reached memory is recorded, not checked.


## Turn 1

**User:** The goal of this project: plan our school's vegetable garden, what grows in each raised bed and what we harvest.


**Agent:** Got it. We can track the beds and the harvest here.

## Turn 2

**User:** We decided as a team: no chemical pesticides in the garden at all, only organic methods.


**Agent:** Noted, organic methods only, no chemical pesticides.

## Turn 3

**User:** The harvest report goes out every Friday, to the parents' group.

**Agent:** Okay, a harvest report every Friday for the parents' group.

## Turn 4

**User:** For all my chats, write dates as YYYY-MM-DD.

**Agent:** Understood, dates as YYYY-MM-DD.

## Turn 5

**User:** Today we picked 14 radishes.

**Agent:** Nice, the radishes were quick.

## Turn 6

**User:** Change of plan: the harvest report now goes out on Mondays, not Fridays. Still to the parents' group.


**Agent:** Okay, the harvest report now goes out on Mondays to the parents' group.

*Memory refreshed: 4 added*

## Database at the end

> SELECT group_concat(section || ': ' || content, char(10)) FROM _jenab_memory WHERE content <> ''

conventions: The harvest report is sent to the parents' group every Monday.
decisions: Use only organic methods and no chemical pesticides in the garden.
goals: Plan the school's vegetable garden, including plant placement in raised beds and harvest tracking.

> SELECT group_concat(section || ': ' || content, char(10)) FROM _jenab_memory WHERE content <> ''

conventions: The harvest report is sent to the parents' group every Monday.
decisions: Use only organic methods and no chemical pesticides in the garden.
goals: Plan the school's vegetable garden, including plant placement in raised beds and harvest tracking.

> SELECT group_concat(section || ': ' || content, char(10)) FROM _jenab_memory WHERE content <> ''

conventions: The harvest report is sent to the parents' group every Monday.
decisions: Use only organic methods and no chemical pesticides in the garden.
goals: Plan the school's vegetable garden, including plant placement in raised beds and harvest tracking.

> SELECT count(*) FROM _jenab_memory WHERE content LIKE '%Friday%' AND content NOT LIKE '%Monday%'

0

> SELECT count(*) FROM _jenab_memory WHERE content LIKE '%14 radish%'

0

## User memory at the end

preferences: Dates should be written in YYYY-MM-DD format.
