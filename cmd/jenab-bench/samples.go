package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// The text samples of SPIKE-048 item 9: each is a chat "Sample NN" whose
// answer is the sample as Markdown, with the sample also in the composer
// as a draft. -samples seeds them and takes a picture of each chat in the
// app, at the same window size each time.

var samples = []string{
	// 1 Persian
	"این یک پاراگراف فارسی است که برای آزمایش شکل حروف، فاصله‌ها و شکستن سطرها نوشته شده است. متن باید از راست به چپ خوانده شود و حروف به هم بچسبند، درست مثل وقتی که در مرورگر دیده می‌شود.",
	// 2 English
	"This is an English paragraph to check kerning, ligatures like fi and fl, and how long lines wrap at the edge of the message. It should read left to right with nothing odd about the spacing.",
	// 3 Persian with English words
	"برای اجرای برنامه، دستور go build را بزنید و بعد فایل jenab.exe را باز کنید. اگر خطای permission denied دیدید، پوشهٔ Settings را بررسی کنید.",
	// 4 English with Persian words
	"The user wrote «سلام دنیا» in the first message, then asked for the word کتاب‌خانه in the table header.",
	// 5 Persian digits, Latin digits and punctuation
	"قیمت ۱۲٬۵۰۰ تومان است (یعنی 12,500)، تاریخ ۱۴۰۵/۰۸/۲۵ و ساعت ۱۰:۴۲؛ آیا درست است؟ «بله»!",
	// 6 ZWNJ and joining
	"می‌خواهم کتاب‌ها را به کتاب‌خانه‌ها برگردانم؛ نمی‌دانم آن‌ها کجا هستند. خانه‌ای بزرگ‌تر، بهترین‌ها.",
	// 7 Brackets and mirroring in RTL
	"تابع (ورودی) را با [آرایه] و {نقشه} صدا بزنید: f(x) = x + 1 و سپس <پایان>.",
	// 8 Heading and paragraph, Persian
	"## گزارش هفتگی\n\nاین هفته **سه کار** انجام شد و *یکی* ماند. برای جزئیات به بخش بعد نگاه کنید.",
	// 9 Persian quote
	"> هر که را صبر نیست، حکمت نیست.\n> — سعدی\n\nاین جمله از گلستان است.",
	// 10 English quote with Persian
	"> The best way out is always through.\n> یعنی: بهترین راه بیرون رفتن، گذشتن از میان است.\n\nA quote, then a line after it.",
	// 11 Code with tabs
	"```go\nfunc main() {\n\tfor i := range 3 {\n\t\tfmt.Println(\"سلام\", i)\t// greet\n\t}\n}\n```",
	// 12 Inline code in Persian
	"مقدار `max_tokens` را روی `4096` بگذارید و سپس `jenab --reset` را اجرا کنید.",
	// 13 English table
	"| Model | Context | Price |\n|---|---:|:---:|\n| Flash Lite | 1M | low |\n| Pro | 2M | high |\n| Local | 32K | free |",
	// 14 Persian table
	"| نام | سن | شهر |\n|---|---|---|\n| علی | ۳۲ | تهران |\n| مریم | ۲۸ | شیراز |\n| Sara | 41 | Berlin |",
	// 15 Persian bullet list
	"- خرید نان\n- پرداخت قبض برق\n- تماس با **دکتر** ساعت ۵\n- خواندن فصل ۳ کتاب",
	// 16 Numbered list, mixed
	"1. Open the project\n2. پروژه را باز کنید و فایل `notes.md` را بخوانید\n3. Run the tests\n4. نتیجه را گزارش دهید",
	// 17 Nested list, English
	"- Fruits\n  - Apples\n  - Pears\n    - Conference\n- Vegetables\n  1. Carrots\n  2. Leeks",
	// 18 Persian with a link and an email
	"برای اطلاعات بیشتر به [صفحهٔ راهنما](https://example.com/help) بروید یا به help@example.com ایمیل بزنید.",
	// 19 Diacritics and Arabic
	"مُحَمَّد گفت: «إِنَّ مَعَ الْعُسْرِ يُسْرًا». کَسرِه، فَتحه و ضَمّه باید بالای حروف درست بنشینند.",
	// 20 Emphasis, emoji and mixed direction in one line
	"**مهم:** نسخهٔ 2.4.1 منتشر شد 🎉 — *please* update before جمعه، ساعت 18:00 CET.",
}

func sampleTitle(i int) string { return fmt.Sprintf("Sample %02d", i+1) }

const shotW, shotH = 1280, 800

// runSamples pictures each sample chat in the app.
func runSamples(o options, exe string, h home) error {
	ctx := context.Background()
	dir := filepath.Join(o.out, "samples")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	a, err := startNative(exe, h, filepath.Join(o.out, "native-samples.log"))
	if err != nil {
		return err
	}
	defer a.quit()
	if err := a.until(ctx, "the projects", 30*time.Second, func(s benchState) bool { return s.Loaded }); err != nil {
		return err
	}
	a.call(ctx, "POST", "/size", url.Values{"w": {fmt.Sprint(shotW)}, "h": {fmt.Sprint(shotH)}}, nil)
	a.call(ctx, "POST", "/front", nil, nil)
	a.call(ctx, "POST", "/open", url.Values{"title": {""}}, nil)
	if err := a.until(ctx, "the Mother chat", 20*time.Second, func(s benchState) bool { return s.Open != "" }); err != nil {
		return err
	}
	for i, s := range samples {
		if err := a.call(ctx, "POST", "/open", url.Values{"title": {sampleTitle(i)}, "draft": {s}}, nil); err != nil {
			return err
		}
		time.Sleep(800 * time.Millisecond)
		p, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("native-%02d.png", i+1)))
		if err := a.call(ctx, "POST", "/shot", url.Values{"path": {p}}, nil); err != nil {
			return err
		}
	}
	if err := a.quit(); err != nil {
		return err
	}
	fmt.Println("samples in", dir)
	return nil
}
