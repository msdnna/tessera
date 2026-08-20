package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

class MarkdownEditTest {
    // ── wrapping a selection ─────────────────────────────────────────────────

    @Test
    fun `typing a pair char over a selection wraps it and keeps it selected`() {
        val e = wrapSelection("hello world", 6, 11, '(')!!
        assertThat(e.text).isEqualTo("hello (world)")
        assertThat(e.start to e.end).isEqualTo(7 to 12)
    }

    @Test
    fun `symmetric marks wrap with themselves`() {
        assertThat(wrapSelection("bold", 0, 4, '*')!!.text).isEqualTo("*bold*")
        assertThat(wrapSelection("q", 0, 1, '"')!!.text).isEqualTo("\"q\"")
    }

    @Test
    fun `a collapsed caret or an unknown char is not a wrap`() {
        assertThat(wrapSelection("abc", 1, 1, '(')).isNull()
        assertThat(wrapSelection("abc", 0, 3, 'x')).isNull()
    }

    // ── indent / outdent ─────────────────────────────────────────────────────

    @Test
    fun `indent shifts every line the selection touches`() {
        // Selection spans lines 1-2; the ends move by different amounts because
        // `end` sits past two inserted indents and `start` past one.
        val e = indentLines("- a\n- b\n- c", 1, 6)
        assertThat(e.text).isEqualTo("  - a\n  - b\n- c")
        assertThat(e.start to e.end).isEqualTo(3 to 10)
    }

    @Test
    fun `outdent strips up to one indent unit per line`() {
        val e = outdentLines("    - a\n  - b", 4, 11)!!
        assertThat(e.text).isEqualTo("  - a\n- b")
        assertThat(e.start to e.end).isEqualTo(2 to 7)
    }

    @Test
    fun `outdent at column 0 does nothing`() {
        assertThat(outdentLines("- a\n- b", 0, 7)).isNull()
    }

    @Test
    fun `outdent never drags the caret before the line start`() {
        // One space only: the caret sat at column 1, so it lands on column 0
        // instead of running back into the previous line.
        val e = outdentLines(" x", 1, 1)!!
        assertThat(e.text).isEqualTo("x")
        assertThat(e.start to e.end).isEqualTo(0 to 0)
    }

    // ── line prefixes ────────────────────────────────────────────────────────

    @Test
    fun `a literal prefix goes on every touched line and the block stays selected`() {
        val e = linePrefixLines("a\nb", 0, 3, "> ")
        assertThat(e.text).isEqualTo("> a\n> b")
        assertThat(e.start to e.end).isEqualTo(0 to 7)
    }

    @Test
    fun `an ordered list numbers the lines from one`() {
        val e = linePrefixLines("a\nb\nc", 0, 5, orderedListPrefix)
        assertThat(e.text).isEqualTo("1. a\n2. b\n3. c")
    }

    // ── auto-close ───────────────────────────────────────────────────────────

    @Test
    fun `a bracket at the end of the text inserts its pair with the caret inside`() {
        val act = autoClose("f", 1, '(') as AutoClose.Insert
        assertThat(act.edit.text).isEqualTo("f()")
        assertThat(act.edit.start).isEqualTo(2)
    }

    @Test
    fun `typing the closer that is already there steps over it`() {
        val act = autoClose("f()", 2, ')') as AutoClose.StepOver
        assertThat(act.caret).isEqualTo(3)
    }

    @Test
    fun `a bracket before existing text stays literal`() {
        // Pairing here would trap "text" inside the brackets.
        assertThat(autoClose("text", 0, '(')).isNull()
    }

    @Test
    fun `an apostrophe inside a word stays literal`() {
        assertThat(autoClose("don", 3, '\'')).isNull()
        assertThat(autoClose("say ", 4, '\'')).isInstanceOf(AutoClose.Insert::class.java)
    }

    @Test
    fun `emphasis marks never auto-close`() {
        // They wrap a selection, but pairing them per keystroke turns "a * b"
        // into "a ** b".
        assertThat(autoClose("a ", 2, '*')).isNull()
        assertThat(autoClose("a ", 2, '_')).isNull()
        assertThat(autoClose("a ", 2, '<')).isNull()
    }

    @Test
    fun `backspace between an empty pair removes both halves`() {
        val e = deletePair("f()", 2)!!
        assertThat(e.text).isEqualTo("f")
        assertThat(e.start).isEqualTo(1)
    }

    @Test
    fun `backspace next to a non-pair is left alone`() {
        assertThat(deletePair("ab", 1)).isNull()
        assertThat(deletePair("()", 0)).isNull()
    }

    // ── Enter: fences and list continuation ──────────────────────────────────

    @Test
    fun `enter after an opening fence closes it below`() {
        val e = handleEnter("```kt", 5)!!
        assertThat(e.text).isEqualTo("```kt\n\n```")
        assertThat(e.start).isEqualTo(6) // on the empty line between the fences
    }

    @Test
    fun `an already closed fence gets an ordinary newline`() {
        assertThat(handleEnter("```\ncode\n```", 3)).isNull()
    }

    @Test
    fun `enter carries the bullet marker to the next line`() {
        val e = handleEnter("- a", 3)!!
        assertThat(e.text).isEqualTo("- a\n- ")
        assertThat(e.start).isEqualTo(6)
    }

    @Test
    fun `ordered markers increment and keep their delimiter and indent`() {
        assertThat(handleEnter("  3) x", 6)!!.text).isEqualTo("  3) x\n  4) ")
    }

    @Test
    fun `a checked box carries over unchecked`() {
        assertThat(handleEnter("- [x] done", 10)!!.text).isEqualTo("- [x] done\n- [ ] ")
    }

    @Test
    fun `enter on an empty item ends the list`() {
        val e = handleEnter("- a\n- ", 6)!!
        assertThat(e.text).isEqualTo("- a\n")
        assertThat(e.start).isEqualTo(4)
    }

    @Test
    fun `enter on plain text is an ordinary newline`() {
        assertThat(handleEnter("plain", 5)).isNull()
    }

    // ── applyTyping: re-deriving the rule from what the IME already did ──────

    @Test
    fun `a char typed over a selection is turned back into a wrap`() {
        val e = applyTyping("hello world", 6, 11, "hello (", 7)!!
        assertThat(e.text).isEqualTo("hello (world)")
    }

    @Test
    fun `a char typed at a caret auto-closes`() {
        val e = applyTyping("f", 1, 1, "f(", 2)!!
        assertThat(e.text).isEqualTo("f()")
        assertThat(e.start).isEqualTo(2)
    }

    @Test
    fun `moving the caret without editing is not a keystroke`() {
        // Same text, different selection — nothing was typed, so no rule applies.
        assertThat(applyTyping("f()", 2, 2, "f()", 3)).isNull()
    }

    @Test
    fun `a typed closer next to its twin does not double it`() {
        val e = applyTyping("f()", 2, 2, "f())", 3)!!
        assertThat(e.text).isEqualTo("f()")
        assertThat(e.start).isEqualTo(3)
    }

    @Test
    fun `a newline goes through list continuation`() {
        val e = applyTyping("- a", 3, 3, "- a\n", 4)!!
        assertThat(e.text).isEqualTo("- a\n- ")
    }

    @Test
    fun `backspace between a pair deletes both halves`() {
        val e = applyTyping("f()", 2, 2, "f)", 1)!!
        assertThat(e.text).isEqualTo("f")
    }

    @Test
    fun `an ordinary keystroke is left to the platform`() {
        assertThat(applyTyping("ab", 2, 2, "abc", 3)).isNull()
    }

    @Test
    fun `a paste or a multi-char change is left to the platform`() {
        assertThat(applyTyping("", 0, 0, "pasted", 6)).isNull()
        assertThat(applyTyping("abcd", 4, 4, "ab", 2)).isNull()
    }
}
