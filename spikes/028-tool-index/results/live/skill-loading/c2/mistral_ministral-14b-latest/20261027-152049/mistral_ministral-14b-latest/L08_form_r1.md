# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"database-design"}

*Skill loaded: database-design*

> result load_skill:
> 

**Agent:** To add a form for manually adding a news item, we need to:

1. **Design the form structure**: The form will include fields for `url`, `date`, `title`, and `reason`.
2. **Save the form configuration**: This will create a new form view in the project.

Let me draft the YAML configuration for the form and save it.

> call save_form {"type":"form","id":"add_news_item","title":"Add news item by hand","fields":[{"name":"url","label":"URL","type":"text","required":true},{"name":"date","label":"Date","type":"text","required":true,"format":"date"},{"name":"title","label":"Title","type":"text","required":true},{"name":"reason","label":"Reason","type":"text","required":false}]}

> result save_form (error):
> there is no tool named save_form

**Agent:** The tool to save a form view is called **`save_view`**. I will save the form configuration using this tool.

> call save_view {"type":"form","id":"add_news_item","title":"Add news item by hand","fields":[{"name":"url","label":"URL","type":"text","required":true},{"name":"date","label":"Date","type":"text","required":true,"format":"date"},{"name":"title","label":"Title","type":"text","required":true},{"name":"reason","label":"Reason","type":"text","required":false}]}

> result save_view (error):
> there is no tool named save_view

**Agent:** It seems I made an error in the tool name. The correct tool to save a **form view** is **`save_form`**, but it appears it is not available in the current tool list.

However, the **`save_view`** tool is intended for saving **table or chart views**, not forms. Since forms are a specific type of view, I will check if there is a way to save a form using the **`save_form`** tool or if we need to adjust the approach.

Let me verify the available tools and confirm the correct way to save a form.

> call describe_tool {"name":"save_form"}

> result describe_tool:
> save_form
> Validate and save a form view config (full YAML, type: form). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete form config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call save_form {"yaml":"type: form\nid: add_news_item\ntitle: \"Add news item by hand\"\nfields:\n  - name: url\n    label: \"URL\"\n    type: text\n    required: true\n  - name: date\n    label: \"Date\"\n    type: text\n    required: true\n    format: date\n  - name: title\n    label: \"Title\"\n    type: text\n    required: true\n  - name: reason\n    label: \"Reason\"\n    type: text\n    required: false"}

> result save_form (error):
> there is no tool named save_form

**Agent:** It appears there is a persistent issue with the tool name recognition. However, based on the tool description, the correct tool is indeed **`save_form`**, but it is not being recognized in the current environment.

Given this, I will proceed by **manually constructing the form configuration** and then attempt to save it using the correct tool name again. If this fails, I will explore alternative approaches, such as saving it as a **YAML file** or using another method to achieve the same result.

Let me try again with the correct tool name and format.

> call call_tool→save_form {"yaml":"type: form\nid: add_news_item_by_hand\ntitle: \"Add news item by hand\"\nfields:\n  - name: url\n    label: \"URL\"\n    type: text\n    required: true\n  - name: date\n    label: \"Date\"\n    type: text\n    required: true\n    format: date\n  - name: title\n    label: \"Title\"\n    type: text\n    required: true\n  - name: reason\n    label: \"Reason\"\n    type: text\n    required: false"}

> result save_form:
> saved form view (revision 1).

**Agent:** The form **"Add news item by hand"** has been successfully saved to the project. You can now use it to manually add news items with the fields: **URL, Date, Title, and Reason**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 23 | 2176 | 0 | 13 | tool_use | 0.8 s |  |
| 2 | 125 | 2176 | 0 | 208 | tool_use | 2.8 s |  |
| 3 | 2518 | 0 | 0 | 158 | tool_use | 9.7 s |  |
| 4 | 509 | 2176 | 0 | 132 | tool_use | 2.5 s |  |
| 5 | 706 | 2176 | 0 | 130 | tool_use | 2.3 s |  |
| 6 | 207 | 2816 | 0 | 257 | tool_use | 9.5 s |  |
| 7 | 1116 | 2176 | 0 | 42 | end | 1.2 s |  |
