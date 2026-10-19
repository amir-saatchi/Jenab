package main

import (
	"context"
	"fmt"
	"sort"

	"burrow/spikes/semanticrecall/builtin"
)

// A provisional Persian check: 10 queries, each with one target passage, in a
// pool that also holds near-topic distractors. Queries are paraphrases: they
// share few or no words with their target. The English queries ask the same
// things, to see the cross-language case too. It picks between the fallback
// models only until the real corpus confirms the pick.
var faPassages = []string{
	// 0-9: targets, in the order of the queries
	"مستأجر می‌تواند با سه ماه اطلاع کتبی قرارداد اجاره را پایان دهد.",
	"در فروشگاه ما قیمت‌ها برای مشتریان اروپایی به یورو نمایش داده می‌شود.",
	"جلسه هفتگی تیم از این پس پنجشنبه‌ها ساعت ده صبح برگزار می‌شود.",
	"رمز عبور باید دست‌کم دوازده نویسه داشته باشد و هر نود روز عوض شود.",
	"پرواز ما به استانبول ساعت شش و نیم صبح از فرودگاه امام خمینی است.",
	"دکتر گفت این قرص را روزی دو بار بعد از غذا بخورم.",
	"کارمندان در سال بیست‌وشش روز مرخصی با حقوق دارند.",
	"سرور پایگاه داده دیشب به‌خاطر پر شدن دیسک از کار افتاد.",
	"برای کیک شکلاتی دویست گرم کره و سه تخم‌مرغ لازم است.",
	"قبض برق این ماه نسبت به ماه قبل دو برابر شده است.",
	// distractors on near topics
	"موجر می‌تواند اجاره را هر سال حداکثر ده درصد افزایش دهد.",
	"ودیعه آپارتمان پس از تحویل کلید ظرف یک ماه بازگردانده می‌شود.",
	"در فروشگاه آمریکایی ما همه قیمت‌ها به دلار است.",
	"هزینه ارسال برای سفارش‌های بالای پنجاه یورو رایگان است.",
	"جلسه با مشتری به هفته آینده موکول شد.",
	"نام کاربری باید با یک حرف شروع شود.",
	"قطار تهران به مشهد هر شب ساعت نه حرکت می‌کند.",
	"این شربت را برای سرفه کودکان تجویز کرده‌اند.",
	"مرخصی استعلاجی با گواهی پزشک پذیرفته می‌شود.",
	"پشتیبان‌گیری از پایگاه داده هر شب ساعت دو انجام می‌شود.",
	"برای نان بربری آرد، خمیرمایه و کمی شکر لازم است.",
	"قبض گاز را می‌توان از طریق برنامه بانک پرداخت کرد.",
}

var faQueries = []string{
	"چقدر زودتر باید برای تخلیه خانه خبر بدهم؟",
	"مشتری‌های اروپا قیمت را با چه پولی می‌بینند؟",
	"گردهمایی هر هفته گروه چه روزی است؟",
	"گذرواژه چند حرف لازم دارد؟",
	"هواپیمای ترکیه کی بلند می‌شود؟",
	"داروی من را چند بار در روز باید مصرف کنم؟",
	"هر کارگر چند روز تعطیلی سالانه دارد؟",
	"چرا دیتابیس دیشب خراب شد؟",
	"برای پختن کیک شکلاتی چه موادی می‌خواهم؟",
	"چرا صورتحساب الکتریسیته این ماه بالا رفته؟",
}

var enQueries = []string{
	"how much notice do I give before moving out of the flat?",
	"which currency do European customers see prices in?",
	"what day is the weekly team meeting?",
	"how many characters does a password need?",
	"when does the flight to Turkey leave?",
	"how often should I take my medicine?",
	"how many days of paid holiday do staff get?",
	"why did the database go down last night?",
	"what do I need to bake a chocolate cake?",
	"why is this month's electricity bill so high?",
}

// persian runs the check with the given query and passage prefixes.
func persian(ctx context.Context, e *builtin.Embedder, qPrefix, pPrefix string) {
	docs := make([]string, len(faPassages))
	for i, p := range faPassages {
		docs[i] = pPrefix + p
	}
	dv := must(e.Embed(ctx, docs))
	for _, set := range []struct {
		name string
		qs   []string
	}{{"fa -> fa", faQueries}, {"en -> fa", enQueries}} {
		qs := make([]string, len(set.qs))
		for i, q := range set.qs {
			qs[i] = qPrefix + q
		}
		qv := must(e.Embed(ctx, qs))
		at1, at3, mrr := 0, 0, 0.0
		for i, q := range qv {
			type scored struct {
				doc   int
				score float64
			}
			var ranked []scored
			for j, d := range dv {
				ranked = append(ranked, scored{j, cosine(q, d)})
			}
			sort.Slice(ranked, func(a, b int) bool { return ranked[a].score > ranked[b].score })
			for r, s := range ranked {
				if s.doc == i {
					mrr += 1 / float64(r+1)
					if r == 0 {
						at1++
					}
					if r < 3 {
						at3++
					}
					break
				}
			}
			if ranked[0].doc != i {
				fmt.Printf("    miss: %s -> %s\n", short(set.qs[i], 40), short(faPassages[ranked[0].doc], 50))
			}
		}
		n := len(qv)
		fmt.Printf("persian %s: recall@1 %d/%d, recall@3 %d/%d, MRR %.3f\n", set.name, at1, n, at3, n, mrr/float64(n))
	}
}
