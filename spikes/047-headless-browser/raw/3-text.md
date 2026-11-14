## 3. Text from JavaScript-only pages (spike's own pages)

Plain = fetch_page's HTTP path today. Load = right after `open`; idle = after `wait --load networkidle`; settled = after the text stopped changing (polled every 0.5 s, up to 6 s). Times are to the idle read (Jenab's likely recipe: open, wait for network idle, get html), cold = a fresh daemon and browser, warm = the same page again on the running browser. Memory and CPU are peaks over cold and warm for the whole tree (CLI, daemon, browser and its children).

### shell

| Page | What | Plain | Load | Idle | Settled | Phrases | Cold | Warm | Peak working set | Peak private | Peak CPU | Processes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spa-inline | empty root; the article is inline JSON rendered by script | 0 | 725 | 725 | 725 | 2/2 | 2.2 s | 1.4 s | 236 MB | 75 MB | 180% | 8 |
| spa-fetch | empty root; the article comes from a JSON API | 0 | 513 | 513 | 513 | 2/2 | 2.1 s | 1.4 s | 238 MB | 78 MB | 192% | 8 |
| spa-delayed | content appears 2.5 s after load | 0 | 0 | 0 | 725 | 1/1 | 2.1 s | 1.4 s | 237 MB | 74 MB | 167% | 8 |
| spa-hash | a hash router picks the content from the fragment | 0 | 482 | 482 | 482 | 2/2 | 2.1 s | 1.4 s | 237 MB | 73 MB | 199% | 8 |
| shadow | a web component renders the article inside its shadow root | 0 | 0 | 0 | 0 | 0/1 | 2.1 s | 1.4 s | 240 MB | 75 MB | 156% | 8 |
| iframe | the shell loads the article in a same-origin iframe | 0 | 0 | 0 | 0 | 0/1 | 2.1 s | 1.4 s | 254 MB | 76 MB | 221% | 8 |
| persian | a Persian article (RTL, ZWNJ) rendered by script | 0 | 436 | 436 | 436 | 2/2 | 2.1 s | 1.4 s | 234 MB | 74 MB | 205% | 8 |
| table | a price table built from a JSON API | 0 | 472 | 472 | 472 | 3/3 | 2.1 s | 1.4 s | 238 MB | 78 MB | 200% | 8 |
| lazy | the first section on load, four more only when scrolled into view | 0 | 288 | 288 | 288 | 1/2 | 2.1 s | 1.4 s | 235 MB | 75 MB | 201% | 8 |
| skeleton | server HTML has skeleton placeholders; script replaces them after an API call | 18 | 18 | 517 | 517 | 2/2 | 2.0 s | 1.3 s | 240 MB | 75 MB | 207% | 8 |
| canvas | text drawn only on a canvas, as Excalidraw does | 0 | 0 | 0 | 0 | 0/1 | 2.1 s | 1.4 s | 238 MB | 79 MB | 202% | 8 |

### cft

| Page | What | Plain | Load | Idle | Settled | Phrases | Cold | Warm | Peak working set | Peak private | Peak CPU | Processes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spa-inline | empty root; the article is inline JSON rendered by script | 0 | 725 | 725 | 725 | 2/2 | 2.5 s | 1.4 s | 942 MB | 529 MB | 674% | 17 |
| spa-fetch | empty root; the article comes from a JSON API | 0 | 513 | 513 | 513 | 2/2 | 2.4 s | 1.5 s | 935 MB | 513 MB | 683% | 17 |
| spa-delayed | content appears 2.5 s after load | 0 | 0 | 0 | 725 | 1/1 | 2.1 s | 1.4 s | 896 MB | 499 MB | 604% | 16 |
| spa-hash | a hash router picks the content from the fragment | 0 | 482 | 482 | 482 | 2/2 | 2.2 s | 1.4 s | 901 MB | 488 MB | 591% | 17 |
| shadow | a web component renders the article inside its shadow root | 0 | 0 | 0 | 0 | 0/1 | 2.3 s | 1.6 s | 921 MB | 507 MB | 536% | 17 |
| iframe | the shell loads the article in a same-origin iframe | 0 | 0 | 0 | 0 | 0/1 | 2.2 s | 1.6 s | 929 MB | 515 MB | 626% | 17 |
| persian | a Persian article (RTL, ZWNJ) rendered by script | 0 | 436 | 436 | 436 | 2/2 | 2.3 s | 2.0 s | 953 MB | 525 MB | 665% | 17 |
| table | a price table built from a JSON API | 0 | 472 | 472 | 472 | 3/3 | 2.3 s | 1.5 s | 932 MB | 511 MB | 637% | 17 |
| lazy | the first section on load, four more only when scrolled into view | 0 | 288 | 288 | 288 | 1/2 | 2.5 s | 1.5 s | 914 MB | 505 MB | 558% | 16 |
| skeleton | server HTML has skeleton placeholders; script replaces them after an API call | 18 | 18 | 517 | 517 | 2/2 | 2.0 s | 1.3 s | 922 MB | 510 MB | 625% | 16 |
| canvas | text drawn only on a canvas, as Excalidraw does | 0 | 0 | 0 | 0 | 0/1 | 2.5 s | 1.5 s | 928 MB | 516 MB | 649% | 16 |

### chrome

| Page | What | Plain | Load | Idle | Settled | Phrases | Cold | Warm | Peak working set | Peak private | Peak CPU | Processes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spa-inline | empty root; the article is inline JSON rendered by script | 0 | 725 | 725 | 725 | 2/2 | 2.4 s | 1.5 s | 837 MB | 425 MB | 582% | 16 |
| spa-fetch | empty root; the article comes from a JSON API | 0 | 513 | 513 | 513 | 2/2 | 2.6 s | 1.5 s | 845 MB | 431 MB | 596% | 16 |
| spa-delayed | content appears 2.5 s after load | 0 | 0 | 0 | 725 | 1/1 | 2.6 s | 1.5 s | 836 MB | 425 MB | 545% | 16 |
| spa-hash | a hash router picks the content from the fragment | 0 | 482 | 482 | 482 | 2/2 | 2.5 s | 1.4 s | 836 MB | 424 MB | 597% | 16 |
| shadow | a web component renders the article inside its shadow root | 0 | 0 | 0 | 0 | 0/1 | 2.4 s | 1.5 s | 852 MB | 436 MB | 619% | 16 |
| iframe | the shell loads the article in a same-origin iframe | 0 | 0 | 0 | 0 | 0/1 | 2.3 s | 1.4 s | 848 MB | 443 MB | 530% | 16 |
| persian | a Persian article (RTL, ZWNJ) rendered by script | 0 | 436 | 436 | 436 | 2/2 | 2.1 s | 1.5 s | 1135 MB | 768 MB | 713044% | 21 |
| table | a price table built from a JSON API | 0 | 472 | 472 | 472 | 3/3 | 2.0 s | 1.4 s | 845 MB | 436 MB | 684% | 16 |
| lazy | the first section on load, four more only when scrolled into view | 0 | 288 | 288 | 288 | 1/2 | 2.0 s | 1.4 s | 826 MB | 430 MB | 716% | 16 |
| skeleton | server HTML has skeleton placeholders; script replaces them after an API call | 18 | 18 | 517 | 517 | 2/2 | 1.8 s | 1.3 s | 831 MB | 437 MB | 685% | 16 |
| canvas | text drawn only on a canvas, as Excalidraw does | 0 | 0 | 0 | 0 | 0/1 | 2.6 s | 1.5 s | 841 MB | 437 MB | 665% | 17 |

### edge

| Page | What | Plain | Load | Idle | Settled | Phrases | Cold | Warm | Peak working set | Peak private | Peak CPU | Processes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spa-inline | empty root; the article is inline JSON rendered by script | 0 | 725 | 725 | 725 | 2/2 | 2.4 s | 1.5 s | 877 MB | 444 MB | 682% | 20 |
| spa-fetch | empty root; the article comes from a JSON API | 0 | 513 | 513 | 513 | 2/2 | 2.3 s | 1.5 s | 880 MB | 450 MB | 746% | 20 |
| spa-delayed | content appears 2.5 s after load | 0 | 0 | 0 | 725 | 1/1 | 2.4 s | 1.5 s | 870 MB | 441 MB | 660% | 20 |
| spa-hash | a hash router picks the content from the fragment | 0 | 482 | 482 | 482 | 2/2 | 2.4 s | 1.4 s | 878 MB | 448 MB | 703% | 20 |
| shadow | a web component renders the article inside its shadow root | 0 | 0 | 0 | 0 | 0/1 | 2.4 s | 1.5 s | 882 MB | 448 MB | 707% | 20 |
| iframe | the shell loads the article in a same-origin iframe | 0 | 0 | 0 | 0 | 0/1 | 2.5 s | 1.5 s | 888 MB | 458 MB | 698% | 20 |
| persian | a Persian article (RTL, ZWNJ) rendered by script | 0 | 436 | 436 | 436 | 2/2 | 2.4 s | 1.5 s | 883 MB | 453 MB | 781% | 20 |
| table | a price table built from a JSON API | 0 | 472 | 472 | 472 | 3/3 | 2.3 s | 1.5 s | 884 MB | 452 MB | 773% | 20 |
| lazy | the first section on load, four more only when scrolled into view | 0 | 288 | 288 | 288 | 1/2 | 2.3 s | 1.5 s | 1273 MB | 1184 MB | 134494% | 21 |
| skeleton | server HTML has skeleton placeholders; script replaces them after an API call | 18 | 18 | 517 | 517 | 2/2 | 2.2 s | 1.3 s | 872 MB | 442 MB | 677% | 20 |
| canvas | text drawn only on a canvas, as Excalidraw does | 0 | 0 | 0 | 0 | 0/1 | 2.3 s | 1.5 s | 881 MB | 451 MB | 773% | 20 |


### Files outside the state folders

New entries: %TEMP%\88ce5978-39ea-467c-be02-ab0cdeac2068.tmp, %TEMP%\db148878-9c82-4678-b9b5-22bcaa05e136.tmp, %TEMP%\spike045-916634645

No agent-browser or browser process left at the end.
