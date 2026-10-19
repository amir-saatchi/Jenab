# File corpus for SPIKE-033

Twelve synthetic documents, four each in English, German and Persian, written as the Markdown TASK-021 makes from a Word file: a `#` title, numbered headings, lists, pipe tables and footnotes listed at the end. Everything in them is made up. `docs.yaml` lists them, and `queries-files.yaml` holds the 30 file queries.

## Documents
Pages are words / 500. Tokens are bytes / 4, as the harness and the app estimate them. Persian takes about two bytes a letter, so a 400-token chunk holds about 180 Persian words, against about 300 English or 250 German ones.

| File | Lang | Kind | Words | Pages | Tokens | Longest table (rows) | Footnotes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| employee-handbook-en.md | en | staff handbook | 14,807 | 29.6 | 21,445 | 51 | 8 |
| heat-pump-manual-en.md | en | product manual | 5,488 | 11.0 | 7,559 | 46 | 5 |
| library-report-en.md | en | annual report | 3,623 | 7.2 | 5,360 | 11 | 4 |
| board-minutes-en.md | en | meeting minutes | 1,741 | 3.5 | 2,534 | 10 | 2 |
| quality-handbook-de.md | de | quality handbook | 14,732 | 29.5 | 26,749 | 67 | 9 |
| rental-contract-de.md | de | rental contract | 4,539 | 9.1 | 8,156 | 31 | 5 |
| work-contract-de.md | de | work contract | 3,035 | 6.1 | 5,969 | 5 | 4 |
| club-minutes-de.md | de | meeting minutes | 1,726 | 3.5 | 3,079 | 17 | 2 |
| irrigation-report-fa.md | fa | research report | 14,290 | 28.6 | 32,302 | 52 | 8 |
| accounting-guide-fa.md | fa | software manual | 4,377 | 8.8 | 9,793 | 36 | 3 |
| renovation-contract-fa.md | fa | building contract | 2,651 | 5.3 | 6,065 | 21 | 2 |
| school-minutes-fa.md | fa | meeting minutes | 1,326 | 2.7 | 3,185 | 8 | 1 |
| **Total** | | | **72,335** | **144.7** | **132,201** | | |

By language: English 51.3 pages, German 48.1, Persian 45.3. The longest table in each long document passes 400 tokens, so the heading chunker must split it by rows: the travel allowances (handbook 5.8), the error codes (heat pump 8.2), the Prüfmittelliste (Anhang A) and the monthly water use (پیوست ۱).

## Distractors
Each target has text nearby on the same topic, in other words:
- **Rental and work contract (German):**
  - **Notice:** both have a notice period of three months to the end of a month. The rental contract has the landlord's longer periods too, and the work contract has two weeks during probation.
  - **Payments:** the rent is due by the third working day to the landlord's account, and the pay on the last bank working day.
  - **Money held back:** the rental contract has the deposit and the work contract a contract penalty.
- **Other documents in the same language:**
  - **Club minutes:** the Kündigung of a membership (to 31 December, six weeks' notice), next to both contracts.
  - **Renovation contract (Persian):** a guarantee of six months for electrical work next to the twelve months for plumbing, a retention deposit, and a cap on the delay penalty.
- **Inside the same file:**
  - **Employee handbook:**
    - **Notice:** a week's notice during probation and the statutory scale after five years.
    - **Budgets and replacement:** the £3,000 team training budget, conference travel on the travel budget, and phones replaced every two years.
  - **Heat pump manual:**
    - **Error codes:** E06, a fan speed sensor fault, sits next to the target row E07.
    - **Frost and maintenance:** the anti-freeze function while the unit has power, normal ice during defrost, and the maintenance table's other intervals.
  - **Library report:** audiobooks up 11 percent, Northgate's Saturday hours, and the other branches closed on Sundays.
  - **Board minutes:** the community hall solar array (approved), a wind turbine share offer and a battery study.
  - **Quality handbook:** the CMM calibrated yearly by an outside lab, internal defect reports in 48 hours, supplier 8D reports in ten working days, and suppliers of the third class with no fixed cycle.
  - **Irrigation report:** a 31 percent saving in the first year, 24 percent for surface drip, a yield gain under 9 percent, and the other barriers (know-how, clogged drippers, small plots).
  - **Accounting guide:** manual backup, restore, changing your own password, and the lock after five wrong tries.
  - **School minutes:** the sixth grade's museum visit, paid from the school budget.
- **Across languages:**
  - **Leave and notice:** the English handbook's rules are near the German work contract's.
  - **Penalties:** the German contract penalty is near the Persian delay penalty.
  - **Deposits:** the Persian retention deposit is near the German rental deposit.
- **No-answer queries:** the documents leave these topics out. Some no-answer queries sit close to real text on purpose, so the score cutoff is tested on near misses:
  - **Close to a real text:** a TÜV lift inspection (near the quality handbook's inspection intervals), an EV wallbox warranty (near the heat pump warranty) and student vaccination (near the school minutes).
  - **Clearly absent:** employee stock options, a shop's returns policy, a home loan and a flight from Tehran to Mashhad.

## Queries
30 queries, as an agent would write a `bucket_search` query. Each target quote is a sentence or two (at most 260 bytes, so 65 tokens), occurs once in its file and was checked with a small Go program that reads both YAML files.

| Query lang | same_words | paraphrase | cross_language | no_answer | Total |
| --- | --- | --- | --- | --- | --- |
| en | 3 | 3 | 2 | 2 | 10 |
| de | 3 | 2 | 3 | 2 | 10 |
| fa | 2 | 3 | 2 | 3 | 10 |
| **Total** | **8** | **8** | **7** | **7** | **30** |

Cross-language pairs (query → document): en → de, en → fa, de → en (2), de → fa, fa → en, fa → de.

Targets by document: three each in the employee handbook, the heat pump manual and the quality handbook; two each in the library report, the rental contract, the irrigation report, the accounting guide and the renovation contract; one each in the board minutes, the work contract, the club minutes and the school minutes. Eight targets are in a long document. One target is a table row: E07 in the heat pump manual.

Paraphrase queries share no word with their target sentence, including small words like "the" or «را». The text around the target may still share words with them, as real text does.

## Format notes
- **Text:** UTF-8, NFC, LF line endings, no BOM.
- **Persian:** keeps ZWNJ, uses Persian ی and ک, writes the ezafe after ه as هٔ, and uses Persian digits. Latin text appears only for key names, error codes, file names and the `1.` of numbered lists.
- **Tables:** have no merged cells and no line breaks in a cell.
