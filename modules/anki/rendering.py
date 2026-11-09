"""Preserve Anki template rendering, typed answers and media references."""
import html
import re
from anki.sound import SoundOrVideoTag, TTSTag

TYPING = re.compile(r"\[\[type:(.*?)\]\]")
PLAY = re.compile(r"\[anki:play:([qa]):(\d+)\]")


def review(engine, typed=""):
    gui = engine.runtime.gui
    value = gui.reviewer_get()
    if not value["active"]:
        return value
    card = gui.card
    value["autoplay"] = card.autoplay()
    config=engine.col.decks.config_dict_for_deck_id(card.did)
    value["replayQuestion"] = config.get("replayq", True)
    value["autoAdvance"]={"questionSeconds":config.get("secondsToShowQuestion",0),"answerSeconds":config.get("secondsToShowAnswer",0),
                          "questionAction":config.get("questionAction",0),"answerAction":config.get("answerAction",0),"waitForAudio":config.get("waitForAudio",True)}
    value["mutationKey"] = gui.mutation_key
    value["customScheduling"] = engine.col.get_config("cardStateCustomizer", "") or ""
    value["customApplied"] = gui.custom_applied
    rendered = value["rendered"]
    match = TYPING.search(rendered["questionHtml"])
    value["typing"] = match is not None
    expected = ""
    if match:
        field = match[1]
        if field.startswith("cloze:"):
            expected = engine.col.extract_cloze_for_typing(card.note()[field[6:]], card.ord)
        else:
            expected = card.note()[field]
        from anki.utils import strip_html
        expected = html.unescape(strip_html(expected))
    comparison = engine.col.compare_answer(expected, typed) if match else ""
    rendered["questionHtml"] = TYPING.sub("", rendered["questionHtml"])
    rendered["answerHtml"] = TYPING.sub(lambda _: comparison, rendered["answerHtml"])
    tags = {"q": card.question_av_tags(), "a": card.answer_av_tags()}
    value["speech"] = [{"text": item.field_text, "language": item.lang, "speed": item.speed,
                        "side": side, "voices": item.voices}
                       for side, items in tags.items() for item in items if isinstance(item, TTSTag)]
    def player(found):
        side, index = found[1], int(found[2])
        if index >= len(tags[side]):
            raise ValueError("Card references an unknown audio item")
        item = tags[side][index]
        if isinstance(item, SoundOrVideoTag):
            from urllib.parse import quote
            video = item.filename.lower().endswith((".mp4", ".webm", ".mkv", ".mov", ".avi"))
            tag = "video" if video else "audio"
            return f'<{tag} controls data-side="{side}" preload="metadata" src="{quote(item.filename, safe="")}"></{tag}>'
        if isinstance(item, TTSTag):
            return ""
        raise ValueError("Unknown card audio type")
    rendered["questionHtml"] = PLAY.sub(player, rendered["questionHtml"])
    rendered["answerHtml"] = PLAY.sub(player, rendered["answerHtml"])
    value["mediaDirectory"] = engine.col.media.dir()
    return value
