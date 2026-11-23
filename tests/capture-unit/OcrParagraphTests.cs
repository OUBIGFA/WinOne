using WinOne.Capture.Ocr;

internal static class OcrParagraphTests
{
    private static OcrBlock Line(string text, double x, double y, double width = 400, double height = 20) =>
        new(text, x, y, x + width, y + height);

    private static void Expect(string name, string expected, params OcrBlock[] lines)
    {
        var actual = OcrReadingOrder.Compose(lines);
        if (actual != expected) throw new Exception($"OCR paragraph {name}: expected [{expected}], got [{actual}]");
    }

    internal static void Run()
    {
        Expect("Chinese wrapped paragraph", "这是一段因为截图宽度而换行的文字，复制后应当连续。",
            Line("这是一段因为截图宽度而", 10, 10), Line("换行的文字，复制后应当", 10, 40), Line("连续。", 10, 70, 110));
        Expect("English wrapped paragraph", "This paragraph wraps at the edge of the image. Copy it as continuous text.",
            Line("This paragraph wraps at the", 10, 10), Line("edge of the image. Copy it as", 10, 40), Line("continuous text.", 10, 70, 200));
        Expect("Separated paragraphs", "第一段的上半行和下半行。\n\n第二段独立保留。",
            Line("第一段的上半行", 10, 10), Line("和下半行。", 10, 40, 160), Line("第二段独立保留。", 10, 110, 240));
        Expect("First-line indentation", "首行缩进的自然段应当与接下来的文字自动衔接。",
            Line("首行缩进的自然段", 50, 10, 360), Line("应当与接下来的文字", 10, 40), Line("自动衔接。", 10, 70, 180));
        Expect("Heading boundary", "章节标题\n\n正文第一行继续到第二行。",
            Line("章节标题", 10, 10, 180, 32), Line("正文第一行", 10, 60), Line("继续到第二行。", 10, 90, 240));
        Expect("List items", "1. First item\n2. Second item\n3. Third item",
            Line("1. First item", 10, 10), Line("2. Second item", 10, 40), Line("3. Third item", 10, 70));
        Expect("Chinese list without spaces", "1. 第一项\n2. 第二项",
            Line("1.第一项", 10, 10), Line("2.第二项", 10, 40));
        Expect("Wrapped list item", "1. This item has a long continuation on the next line.\n2. Another item.",
            Line("1. This item has a long", 10, 10), Line("continuation on the next line.", 35, 40, 375), Line("2. Another item.", 10, 70, 200));
        Expect("Source code", "if (ready) {\n    return value;\n}",
            Line("if (ready) {", 10, 10, 250), Line("return value;", 50, 40, 220), Line("}", 10, 70, 15));
        Expect("Two columns", "左栏第一行左栏第二行。\n\n右栏第一行右栏第二行。",
            Line("右栏第二行。", 330, 40, 250), Line("左栏第一行", 10, 10, 250), Line("右栏第一行", 330, 10, 250), Line("左栏第二行。", 10, 40, 250));
        Expect("Punctuation and CJK", "你好，世界。",
            Line("你好", 10, 10, 100), Line("，世界。", 10, 40, 90));
        Expect("Preserve hyphen", "A well-known example.",
            Line("A well-", 10, 10, 200), Line("known example.", 10, 40, 200));
        Expect("Empty", "");
        Expect("Indented next paragraph", "第一段开头延续到第二行。\n\n第二段开头延续到下一行。",
            Line("第一段开头", 50, 10, 360), Line("延续到第二行。", 10, 40, 260),
            Line("第二段开头", 50, 70, 360), Line("延续到下一行。", 10, 100, 260));
        // Layout detection must never lose or duplicate an OCR block while
        // assigning overlapping rows, narrow fragments or changing columns.
        var random = new Random(1124);
        for (int example = 0; example < 150; example++)
        {
            var blocks = Enumerable.Range(0, random.Next(1, 25)).Select(index =>
                Line($"Token{index:D2}X", random.Next(0, 700), random.Next(0, 700), random.Next(5, 230), random.Next(8, 36))).ToArray();
            string text = OcrReadingOrder.Compose(blocks);
            foreach (var block in blocks)
                if (text.Split(block.Text, StringSplitOptions.None).Length != 2)
                    throw new Exception("OCR column grouping lost or repeated " + block.Text);
        }
        Console.WriteLine("PASS: 14 OCR paragraph cases and 150 layout preservation cases");
    }
}
