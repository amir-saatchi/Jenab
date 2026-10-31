# TASK-027: the history cards of the set's follow-ups

Worked out offline as the app does at the follow-up's first request: the project has all eight chats, the follow-up is turn 23, the first request cuts and the window starts at turn 20. Thresholds as frozen on SPIKE-028's chats (min score 5.0, margin 1.25).

| follow-up | test | needs | cards | verdict | best scores (turn score/words) | counted words shared |
|---|---|---|---|---|---|---|
| C1_ctrl | control | turn 21 | 4, 13 | false card | t13 10.8/4, t4 7.0/3, t21 3.1/1, t2 2.4/1 | t4: save that view; t13: save line chart titled; t21: ratio |
| C1_para | paraphrase | turn 9 | 16 | wrong card | t16 7.9/3, t7 4.8/2, t9 4.1/2, t20 3.1/1 | t9: when its; t16: early up when |
| C1_repeat | repeat | turn 4 | 4 | needed turn carded | t4 14.8/5, t1 3.1/1, t13 3.1/1, t20 3.1/1 | t4: that weekly volatility table has |
| C2_ctrl | control | none | 1, 8 | false card | t8 6.8/3, t1 5.4/3, t16 4.8/2, t3 4.0/2 | t1: which have job; t8: have more job |
| C2_para | paraphrase | turn 8 | 8, 12 | needed turn carded, with another | t12 7.0/3, t8 6.6/3, t3 3.9/2, t4 3.1/1 | t8: that for applications; t12: for interview ones |
| C2_repeat | repeat | turn 3 | 3 | needed turn carded | t3 14.7/6, t8 3.5/2, t9 3.5/2, t17 3.5/2 | t3: job scan for remote developer jobs |
| C3_ctrl | control | turn 21 | none | no card (right) | t5 4.1/2, t7 2.4/1, t10 2.4/1, t17 2.4/1 | t21: what |
| C3_para | paraphrase | turn 10 | none | no card | t1 4.4/2, t7 4.4/2, t10 2.4/1, t22 2.4/1 | t10: that |
| C3_repeat | repeat | turn 5 | 5 | needed turn carded | t5 15.8/6, t10 3.9/2, t22 2.4/1, t6 2.0/1 | t5: make spending against bar chart as |
| C4_ctrl | control | none | 8 | false card | t8 6.3/3, t4 4.8/2, t6 3.9/2, t2 3.2/2 | t8: what was my |
| C4_para | paraphrase | turn 12 | 6 | wrong card | t6 9.4/4, t22 4.8/2, t5 3.9/2, t12 3.2/2 | t6: of per first each; t12: of per |
| C4_repeat | repeat | turn 6 | 6 | needed turn carded | t6 15.7/7, t5 4.1/2, t17 4.1/2, t12 3.7/2 | t6: make pace per route table with average |
| C5_ctrl | control | none | 3, 12 | false card | t3 8.7/4, t12 6.3/3, t1 2.4/1, t4 1.5/1 | t3: log today liters b2; t12: log today liters |
| C5_para | paraphrase | turn 11 | 11, 17 | needed turn carded, with another | t11 5.1/2, t17 5.1/2, t1 3.1/1, t16 3.1/1 | t11: night was; t17: of high |
| C5_repeat | repeat | turn 4 | 1, 4 | needed turn carded, with another | t4 9.4/4, t1 7.9/3, t12 5.4/3, t17 4.8/2 | t1: raised beds r1; t4: make liters per chart |
| C6_ctrl | control | turn 21 | none | no card (right) | t1 2.4/1, t3 2.4/1 |  |
| C6_para | paraphrase | turn 9 | 5, 18 | wrong card | t5 6.2/2, t18 6.1/3, t9 4.8/2, t15 3.7/2 | t5: آخر پول; t9: رو برای; t18: هفته برای هم |
| C6_repeat | repeat | turn 3 | 3, 9 | needed turn carded, with another | t3 11.2/5, t9 7.4/4, t10 5.1/2, t16 4.1/2 | t3: جدول اجارهی هر مربع بساز; t9: جدول اجارهی هر برای |
| C7_ctrl | control | none | 10 | false card | t10 5.5/2, t6 4.4/2, t5 3.1/1, t2 2.4/1 | t10: من دارد |
| C7_para | paraphrase | turn 13 | none | no card | t1 2.0/1, t7 2.0/1, t10 2.0/1, t14 2.0/1 | t13: رو |
| C7_repeat | repeat | turn 7 | 7 | needed turn carded | t7 11.3/4, t14 5.1/2, t1 2.0/1, t10 2.0/1 | t7: نمودار ماه برای بساز |
| C8_ctrl | control | turn 21 | none | no card (right) | t13 4.6/2, t1 2.0/1, t3 2.0/1, t17 2.0/1 |  |
| C8_para | paraphrase | turn 10 | 19 | wrong card | t19 6.3/3, t13 3.9/2, t16 3.2/2, t20 3.2/2 | t10: که; t19: این مدت بالا |
| C8_repeat | repeat | turn 5 | 5 | needed turn carded | t5 14.5/6, t17 6.4/3, t13 4.6/2, t12 4.4/2 | t5: نمودار میانگین متحرک روزه هم بساز |

Totals:

- control: false card: 5
- control: no card (right): 3
- paraphrase: needed turn carded, with another: 2
- paraphrase: no card: 2
- paraphrase: wrong card: 4
- repeat: needed turn carded: 6
- repeat: needed turn carded, with another: 2
