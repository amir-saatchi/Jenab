# TASK-027: candidate card rules, offline

The history-cards set (24 follow-ups, 8 chats in one project) and SPIKE-028's long chats (H1-H5, one chat a project). Needed: the follow-ups whose needed turn is past the cut. Margin 1.25 unless named.

| rule | min score | set: repeats | set: paraphrases | set: wrong only | set: extra card | set: false on controls | dev: needed | dev: wrong | dev: false on controls |
|---|---|---|---|---|---|---|---|---|---|
| frozen | 4 | 8/8 | 2/8 | 5 | 4 | 7/8 | 2/4 | 0 | 0/1 |
| anchor<=2 | 4 | 8/8 | 2/8 | 5 | 3 | 7/8 | 2/4 | 0 | 0/1 |
| anchor<=1 | 4 | 8/8 | 3/8 | 3 | 4 | 5/8 | 2/4 | 0 | 0/1 |
| anchor<=2, 3 words | 4 | 8/8 | 1/8 | 4 | 2 | 4/8 | 2/4 | 0 | 0/1 |
| project idf | 4 | 8/8 | 2/8 | 5 | 6 | 7/8 | 2/4 | 1 | 0/1 |
| project idf, anchor<=2 | 4 | 8/8 | 2/8 | 5 | 5 | 7/8 | 2/4 | 1 | 0/1 |
| stop lists | 4 | 7/8 | 0/8 | 6 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| stop lists, anchor<=2 | 4 | 7/8 | 0/8 | 6 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| window 1 word | 4 | 8/8 | 2/8 | 5 | 4 | 7/8 | 2/4 | 0 | 0/1 |
| window 1 word, anchor<=1 | 4 | 8/8 | 3/8 | 3 | 4 | 5/8 | 2/4 | 0 | 0/1 |
| window 1 word, margin 2 | 4 | 8/8 | 2/8 | 2 | 4 | 7/8 | 1/4 | 0 | 0/1 |
| window 1 word, anchor<=1, margin 2 | 4 | 8/8 | 3/8 | 1 | 4 | 5/8 | 1/4 | 0 | 0/1 |
| stop lists, window 1 word | 4 | 7/8 | 0/8 | 6 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| frozen | 5 | 8/8 | 2/8 | 4 | 4 | 5/8 | 2/4 | 0 | 0/1 |
| anchor<=2 | 5 | 8/8 | 2/8 | 4 | 3 | 5/8 | 2/4 | 0 | 0/1 |
| anchor<=1 | 5 | 8/8 | 2/8 | 4 | 3 | 4/8 | 2/4 | 0 | 0/1 |
| anchor<=2, 3 words | 5 | 8/8 | 1/8 | 4 | 2 | 4/8 | 2/4 | 0 | 0/1 |
| project idf | 5 | 8/8 | 2/8 | 5 | 6 | 7/8 | 2/4 | 1 | 0/1 |
| project idf, anchor<=2 | 5 | 8/8 | 2/8 | 5 | 5 | 7/8 | 2/4 | 1 | 0/1 |
| stop lists | 5 | 7/8 | 0/8 | 4 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| stop lists, anchor<=2 | 5 | 7/8 | 0/8 | 4 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| window 1 word | 5 | 8/8 | 2/8 | 4 | 4 | 5/8 | 2/4 | 0 | 0/1 |
| window 1 word, anchor<=1 | 5 | 8/8 | 2/8 | 4 | 3 | 4/8 | 2/4 | 0 | 0/1 |
| window 1 word, margin 2 | 5 | 8/8 | 2/8 | 2 | 4 | 5/8 | 1/4 | 0 | 0/1 |
| window 1 word, anchor<=1, margin 2 | 5 | 8/8 | 2/8 | 2 | 3 | 4/8 | 1/4 | 0 | 0/1 |
| stop lists, window 1 word | 5 | 7/8 | 0/8 | 4 | 0 | 2/8 | 1/4 | 0 | 0/1 |
| frozen | 6 | 8/8 | 1/8 | 4 | 3 | 4/8 | 2/4 | 0 | 0/1 |
| anchor<=2 | 6 | 8/8 | 1/8 | 4 | 2 | 4/8 | 2/4 | 0 | 0/1 |
| anchor<=1 | 6 | 8/8 | 1/8 | 4 | 2 | 3/8 | 2/4 | 0 | 0/1 |
| anchor<=2, 3 words | 6 | 8/8 | 1/8 | 4 | 2 | 4/8 | 2/4 | 0 | 0/1 |
| project idf | 6 | 8/8 | 2/8 | 5 | 5 | 7/8 | 2/4 | 1 | 0/1 |
| project idf, anchor<=2 | 6 | 8/8 | 2/8 | 5 | 4 | 7/8 | 2/4 | 1 | 0/1 |
| stop lists | 6 | 7/8 | 0/8 | 3 | 0 | 2/8 | 0/4 | 0 | 0/1 |
| stop lists, anchor<=2 | 6 | 7/8 | 0/8 | 3 | 0 | 2/8 | 0/4 | 0 | 0/1 |
| window 1 word | 6 | 8/8 | 1/8 | 4 | 3 | 4/8 | 2/4 | 0 | 0/1 |
| window 1 word, anchor<=1 | 6 | 8/8 | 1/8 | 4 | 2 | 3/8 | 2/4 | 0 | 0/1 |
| window 1 word, margin 2 | 6 | 8/8 | 1/8 | 2 | 3 | 4/8 | 1/4 | 0 | 0/1 |
| window 1 word, anchor<=1, margin 2 | 6 | 8/8 | 1/8 | 2 | 2 | 3/8 | 1/4 | 0 | 0/1 |
| stop lists, window 1 word | 6 | 7/8 | 0/8 | 3 | 0 | 2/8 | 0/4 | 0 | 0/1 |

## Details at min score 5

- frozen: false cards C1_ctrl[4 13] C2_ctrl[1 8] C4_ctrl[8] C5_ctrl[3 12] C7_ctrl[10]; wrong cards C1_para[16] C4_para[6] C6_para[5 18] C8_para[19]
- anchor<=2: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C5_ctrl[3 12] C7_ctrl[10]; wrong cards C1_para[16] C4_para[6] C6_para[5 18] C8_para[19]
- anchor<=1: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C7_ctrl[10]; wrong cards C1_para[16] C4_para[6] C6_para[5] C8_para[19]
- anchor<=2, 3 words: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C5_ctrl[3 12]; wrong cards C1_para[16] C4_para[6] C6_para[18] C8_para[19]
- project idf: false cards C1_ctrl[4 13] C2_ctrl[1 8] C3_ctrl[5] C4_ctrl[6 8] C5_ctrl[3 12] C7_ctrl[6 10] C8_ctrl[13]; wrong cards C1_para[7 16] C3_para[1 7] C4_para[6] C6_para[5 18] C8_para[19]
- project idf, anchor<=2: false cards C1_ctrl[4 13] C2_ctrl[8 16] C3_ctrl[5] C4_ctrl[6 8] C5_ctrl[3 12] C7_ctrl[6 10] C8_ctrl[13]; wrong cards C1_para[7 16] C3_para[1 7] C4_para[6] C6_para[5 18] C8_para[19]
- stop lists: false cards C1_ctrl[13] C5_ctrl[3 12]; wrong cards C1_para[16] C2_para[12] C5_repeat[1] C6_para[5]
- stop lists, anchor<=2: false cards C1_ctrl[13] C5_ctrl[3 12]; wrong cards C1_para[16] C2_para[12] C5_repeat[1] C6_para[5]
- window 1 word: false cards C1_ctrl[4 13] C2_ctrl[1 8] C4_ctrl[8] C5_ctrl[3 12] C7_ctrl[10]; wrong cards C1_para[16] C4_para[6] C6_para[5 18] C8_para[19]
- window 1 word, anchor<=1: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C7_ctrl[10]; wrong cards C1_para[16] C4_para[6] C6_para[5] C8_para[19]
- window 1 word, margin 2: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C5_ctrl[3 12] C7_ctrl[10]; wrong cards C1_para[16] C6_para[5 18]
- window 1 word, anchor<=1, margin 2: false cards C1_ctrl[4 13] C2_ctrl[8] C4_ctrl[8] C7_ctrl[10]; wrong cards C1_para[16] C6_para[5]
- stop lists, window 1 word: false cards C1_ctrl[13] C5_ctrl[3 12]; wrong cards C1_para[16] C2_para[12] C5_repeat[1] C6_para[5]

## Words of the set's follow-ups

Each counted word (not project-common): the chat's earlier turns with it. Project parts in brackets.

- C1_ctrl (needs 21): save [4 5 9 13 17][34], that [2 4][10], ratio [21][3], as [][8], line [13][5], chart [13][9], view [4][2], titled [13][2], eth [1 6 9 11 16 19 21][10], btc [1 2 4 14 18 19 21][10]
- C1_para (needs 9): could [][1], bitcoin [7][2], get [][3], the [3 4 5 6 7 9 11 12 13 16 17 19 20 21][86], same [20][3], early [16][2], heads [][1], up [16][10], when [6 7 9 16][15], it [4 6 7 9 10 11 16 17 22][33], sinks [][1], well [][2], under [][5], its [9 11][8], recent [][1], top [][2]
- C1_repeat (needs 4): make [13][9], that [2 4][10], weekly [4][7], volatility [4][3], table [4][4], for [1 3 4 7 16 21][42], sol [1 3 5 8 12 13 20][12], too [][2], prices [1][4], has [4][6], the [3 4 5 6 7 9 11 12 13 16 17 19 20 21][86], same [20][3], columns [][1], as [][8], btc [1 2 4 14 18 19 21][10]
- C2_ctrl (needs 0): which [1 2 10 16][15], companies [16][2], have [1 8 19 20][12], more [8][7], than [22][7], one [3 12 13][12], post [1 2 3 5 9 10 11 14 15 19][18], in [3 9 10 12 16 17 18 19 20][55], job [1 3 8][6], posts [1 3 9 10 14 15 19][13], give [][2], the [2 3 8 9 13 14 17 18 19 21 22][86], counts [][1]
- C2_para (needs 8): can [4][2], get [][3], that [8][10], same [18][3], overview [][1], for [3 8 9 12 17][42], applications [7 8 20][5], stuck [][1], after [][4], an [1 3 8 10 12 16 17 21][52], interview [12][3], the [2 3 8 9 13 14 17 18 19 21 22][86], ones [3 12][3], nobody [][1], has [15][6], replied [][1], to [1 2 3 5 6 9 11 13 16 17 20][51], while [][1]
- C2_repeat (needs 3): set [][1], up [2 9 17][10], the [2 3 8 9 13 14 17 18 19 21 22][86], same [18][3], job [1 3 8][6], scan [3][2], for [3 8 9 12 17][42], remote [3 10 14][5], rust [][1], developer [3][4], jobs [3][3]
- C3_ctrl (needs 21): and [1 8 10 13 14 17 20][48], what [4 5 13 21][20], about [7 17][6], july [][3], same [][3], category [5 10][4]
- C3_para (needs 10): can [][2], eating [1 7 14][4], out [1 7][6], get [][3], that [10 22][10], same [][3], weekend [][2], nudge [][1]
- C3_repeat (needs 5): make [5][9], that [10 22][10], spending [1 5 10 17 20][7], against [5][2], budget [1 5 7 10 12 17 20 22][12], bar [5][7], chart [5][9], for [3 5 8 9 14 15 17][42], 2025 [][1], as [5 6 8][8], well [][2]
- C4_ctrl (needs 0): what [2 8 11 14][20], was [8][13], my [2 6 8 14 17][8], longest [][1], run [1 3 4 5 6 10 12 13 18 20][20], in [1 3 5 6 8 10 12 16 20 21][55], october [][2], and [1 4 11 12 16][48], on [3 6 8 9 11 12 14 15 19 21][53], which [4 18][15], route [4 6][4]
- C4_para (needs 12): could [][1], also [][1], get [][3], that [5 22][10], kind [][3], of [5 6 12 16 17][33], digest [][1], per [6 8 12 17][19], calendar [][1], month [18 22][15], on [3 6 8 9 11 12 14 15 19 21][53], the [1 3 5 7 11 12 14 15 16 18 19 21][86], first [6][11], each [6][15]
- C4_repeat (needs 6): make [6 17][9], that [5 22][10], pace [6 16][4], per [6 8 12 17][19], route [4 6][4], table [6][4], again [][2], but [][2], with [6 12 18][28], the [1 3 5 7 11 12 14 15 16 18 19 21][86], average [3 5 6 14][10], heart [3 5 9 14 15 19][7], rate [3 5 9 14 15 19][7], instead [][1], same [][3], rules [][1]
- C5_ctrl (needs 0): log [3 12][13], today [3 12][7], watering [][1], liters [3 4 12 13 15][9], on [4 7 8 12 16 20 22][53], b2 [1 3][4]
- C5_para (needs 11): could [][1], we [1 2 4 5 11 18 22][38], have [1][12], heat [][1], version [][1], of [4 17 21][33], the [1 4 5 6 10 11 13 14 15 16 17 21 22][86], cold [][1], night [11][3], protection [][1], when [5 10 11 14 16 21][15], yesterday [][2], high [17][10], was [5 6 11][13], above [][3], 32 [][1], remind [][1], me [][8], to [1 3 5 11 12 13 15 16 20][51], shade [][1], young [][1], plants [16][2]
- C5_repeat (needs 4): make [4 17][9], that [5][10], liters [3 4 12 13 15][9], per [4][19], bed [1 4 8 11 15 18][10], chart [4 17][9], again [][2], but [][2], only [16][6], for [3 5 11 12 21][42], the [1 4 5 6 10 11 13 14 15 16 17 21 22][86], raised [1][2], beds [1 18][4], r1 [1 12][4], to [1 3 5 11 12 13 15 16 20][51], r4 [8 18][4]
- C6_ctrl (needs 21): سعادتآباد [1 3][4], چطور [][2]
- C6_para (needs 9): میشه [][1], اون [][1], گزارش [][2], آخر [5][5], هفته [18 19][6], رو [9][21], برای [7 9 15 18][12], پول [5][3], پیش [][2], هم [10 15 18][12], راه [][1], بندازی [][1]
- C6_repeat (needs 3): همین [10][3], جدول [3 7 9][7], اجارهی [3 5 9][6], هر [3 5 9 16][14], متر [2 3 6 11 17 18 21][11], مربع [3][3], را [3 4 5 7 9 13 15 18][31], برای [7 9 15 18][12], پونک [1 6 20][5], هم [10 15 18][12], بساز [3 16][7]
- C7_ctrl (needs 0): کدام [2 9][10], نویسنده [][1], بیشترین [5][6], کتاب [1 6 8 9 10 13 17 21][13], را [2 6 7 8 10 13 17 18 19 21][31], در [1 3 4 5 6 7 10 12 13 15 16][55], فهرست [1 6 8][5], من [6 10][6], دارد [10][2]
- C7_para (needs 13): برای [1 7 21][12], انتشارات [][1], نیلوفر [][1], هم [10 14 18][12], همون [][1], پیگیری [][1], روزانه [][3], رو [5 12 13 16 20][21], راه [][1], بنداز [][1]
- C7_repeat (needs 7): همین [14][3], نمودار [7][9], صفحه [3 5 7 8 11 12 15 19 20][16], در [1 3 4 5 6 7 10 12 13 15 16][55], ماه [7][14], را [2 6 7 8 10 13 17 18 19 21][31], برای [1 7 21][12], پارسال [][1], هم [10 14 18][12], بساز [7][7]
- C8_ctrl (needs 21): برای [13][12], خودرو [1 3 17][4], هم [5 7 9 13 16][12], همین [][3], را [3 6 9 10 13 14 15 16 18][31], بگو [][1]
- C8_para (needs 10): یه [][1], لیست [][1], مشابه [][1], هم [5 7 9 13 16][12], میخوام [][1], از [2 5 10 13 14 17 18][28], سهمهایی [][1], که [10 13][15], این [16 18 19 20][11], مدت [19][2], حسابی [][1], بالا [8 9 11 19 20][7], رفتن [][1]
- C8_repeat (needs 5): همین [][3], نمودار [5 17][9], میانگین [5 12][10], متحرک [5][3], 20 [2 5 19 21][51], روزه [5][4], را [3 6 9 10 13 14 15 16 18][31], برای [13][12], خودرو [1 3 17][4], شپنا [1 12 21][4], هم [5 7 9 13 16][12], بساز [5 10 17][7]
