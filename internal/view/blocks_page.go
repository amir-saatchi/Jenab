package view

// The page units: view, heading, text, card, filter, button and image
// blocks (SPEC 5.9).

func registerPage(r *Registry) {
	MustRegister(r, Unit[ViewBlock]{Type: "view", Kind: PageBlock, Schema: schemaFile("view.json"), Check: checkViewBlock})
	MustRegister(r, Unit[HeadingBlock]{Type: "heading", Kind: PageBlock, Schema: schemaFile("heading.json")})
	MustRegister(r, Unit[TextBlock]{Type: "text", Kind: PageBlock, Schema: schemaFile("text.json")})
	MustRegister(r, Unit[CardBlock]{Type: "card", Kind: PageBlock, Schema: schemaFile("card.json"), Container: true})
	MustRegister(r, Unit[FilterBlock]{Type: "filter", Kind: PageBlock, Schema: schemaFile("filter.json"), Check: checkFilterBlock})
	MustRegister(r, Unit[ButtonBlock]{Type: "button", Kind: PageBlock, Schema: schemaFile("button.json"), Check: checkButton})
	MustRegister(r, Unit[ImageBlock]{Type: "image", Kind: PageBlock, Schema: schemaFile("image.json"), Check: checkImage})
}

// checkViewBlock checks that the view exists, and binds its query's
// parameters for the page's filters.
func checkViewBlock(c *Check, b *ViewBlock) {
	if !c.Env() || b.View == "" {
		c.Unbound()
		return
	}
	v, ok := c.ViewRef("/view", b.View)
	if !ok {
		c.Unbound()
		return
	}
	c.Binds(queryParams(v.Query)...)
}

func checkFilterBlock(c *Check, b *FilterBlock) {
	c.ShowFilter("/filter", b.Filter)
}

func checkButton(c *Check, b *ButtonBlock) {
	c.Target("", "a button", b.RunPipeline, b.OpenPage, b.OpenForm)
}

func checkImage(c *Check, b *ImageBlock) {
	c.Object("/image", b.Image)
}
