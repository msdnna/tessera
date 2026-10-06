import { describe, it, expect } from 'vitest'
import { parseQuick } from '@/utils/quickParse'

// The start screen's quick entry (#2960): one line read token by token. What the
// parser must never do is lose words — anything it doesn't recognise stays in
// the title.
const now = new Date(2026, 9, 5, 9, 30) // Mon 5 Oct 2026, 09:30
const people = [
  { user_id: 'u1', name: 'Анна Ким', email: 'anna@x.dev' },
  { user_id: 'u2', name: 'Илья Петров', email: 'ilya@x.dev', gl_username: 'ipetrov' },
  { user_id: 'u3', name: 'Илона Орлова', email: 'ilona@x.dev' },
]
const tags = [{ id: 't1', name: 'Звонки', color: '#0eb0a9' }]
const read = (text) => parseQuick(text, { now, people, tags })

describe('parseQuick', () => {
  it('takes the date, time, priority, tag and person out of the title', () => {
    const r = read('Позвонить клиенту завтра 11:00 !высокий #звонки @анна')
    expect(r.title).toBe('Позвонить клиенту')
    expect(r.time).toBe('11:00')
    expect(r.day.getDate()).toBe(6)
    expect(new Date(r.due).getHours()).toBe(11)
    expect(r.priority).toBe(3)
    expect(r.tags).toEqual([{ name: 'звонки', tag: tags[0] }])
    expect(r.people.map((p) => p.user_id)).toEqual(['u1'])
  })

  it('stores an all-day date as UTC midnight of that local day', () => {
    const r = read('Отчёт пт')
    expect(r.due).toBe(new Date(Date.UTC(2026, 9, 9)).toISOString())
    expect(r.time).toBeNull()
  })

  it('reads a weekday as the coming one, today included', () => {
    expect(read('a пн').day.getDate()).toBe(5) // today is Monday
    expect(read('a вс').day.getDate()).toBe(11)
    expect(read('a friday').day.getDate()).toBe(9)
  })

  it('reads dd.mm, rolling a past date into next year', () => {
    expect(read('a 14.10').day.getMonth()).toBe(9)
    const past = read('a 01.02')
    expect(past.day.getFullYear()).toBe(2027)
    expect(read('a 31.02').day).toBeNull() // no such day — the word stays
    expect(read('a 31.02').title).toBe('a 31.02')
  })

  it('tells a dotted date from a dotted time', () => {
    expect(read('a 14.10').time).toBeNull()
    expect(read('a 11.45').time).toBe('11:45') // no 45th month
  })

  it('puts a bare time on today', () => {
    const r = read('Созвон 16:45')
    expect(r.day.getDate()).toBe(5)
    expect(new Date(r.due).getMinutes()).toBe(45)
  })

  it('knows priorities by word or digit, in both languages', () => {
    expect(read('a !срочно').priority).toBe(4)
    expect(read('a !urgent').priority).toBe(4)
    expect(read('a !2').priority).toBe(2)
    expect(read('a !низкий').priority).toBe(1)
    expect(read('a !0').priority).toBe(0)
    expect(read('a !что-то').title).toBe('a !что-то') // unknown → text
  })

  it('keeps an unknown tag as a new one and an ambiguous person as text', () => {
    const r = read('a #новый @ил')
    expect(r.tags).toEqual([{ name: 'новый', tag: null }])
    expect(r.people).toEqual([]) // Илья and Илона both match «ил»
    expect(r.title).toBe('a @ил')
    expect(read('a @ipetrov').people[0].user_id).toBe('u2')
  })

  it('leaves the rest alone', () => {
    const r = read('Просто заголовок')
    expect(r).toMatchObject({ title: 'Просто заголовок', due: null, priority: null })
  })
})
