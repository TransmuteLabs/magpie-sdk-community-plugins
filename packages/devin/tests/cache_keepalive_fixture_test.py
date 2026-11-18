import struct
import unittest

import cache_keepalive_host_e2e as fixture
from conformance import fields, number_field, text_field, varint


def sampling(maximum):
    return (number_field(1, 1) + number_field(2, maximum)
            + b"\x29" + struct.pack("<d", 0.0) + b"\x41" + struct.pack("<d", 0.0)
            + text_field(9, "cache-stop") + number_field(10, 7)
            + number_field(3, 400) + number_field(7, 40))


def request(maximum, continued=False, choice="Read", parallel=1):
    prompt = text_field(1, "original") + number_field(2, 1) + text_field(3, "prefix")
    payload = text_field(3, prompt)
    if continued:
        extra = text_field(1, "fresh") + number_field(2, 1) + text_field(3, "continue")
        payload += text_field(3, extra)
    payload += text_field(8, sampling(maximum))
    payload += text_field(10, text_field(1, "Read") + text_field(3, '{"type":"object"}'))
    payload += number_field(11, parallel)
    if choice is not None:
        payload += text_field(12, text_field(2, choice))
    return payload + text_field(21, "fixture-model")


class CacheKeepaliveFixtureTests(unittest.TestCase):
    def test_legacy_parser_rejects_valid_fixed32_and_native_fixed64_chunks(self):
        for number, wire, raw in (
            (5, 1, b"\x29" + struct.pack("<d", 0.0)),
            (8, 1, b"\x41" + struct.pack("<d", 0.0)),
            (99, 5, varint(99 << 3 | 5) + struct.pack("<f", 0.5)),
        ):
            with self.subTest(number=number, wire=wire):
                with self.assertRaisesRegex(AssertionError, "unexpected native fixture wire type"):
                    fields(raw)
                self.assertEqual(fixture.sampling_chunks(raw), [(number, wire, raw)])

    def test_template_accepts_native_fixed64_sampling_and_named_tool_choice(self):
        fixture.verify_template(request(64), request(1, continued=True))

    def test_sampling_scan_rejects_invalid_and_truncated_chunks(self):
        for raw in (b"\0", b"\x0b", b"\x0e", b"\x08\x80", b"\x29\0", b"\x45\0",
                    b"\x4a\x03a", b"\x4a\x80"):
            with self.subTest(raw=raw):
                with self.assertRaises(AssertionError):
                    fixture.sampling_chunks(raw)

    def test_template_rejects_changed_sampling_bytes(self):
        paid = request(1, continued=True).replace(
            b"\x29" + struct.pack("<d", 0.0), b"\x29" + struct.pack("<d", 0.5), 1)
        with self.assertRaises(AssertionError):
            fixture.verify_template(request(64), paid)

    def test_template_requires_exact_foreground_sampling(self):
        before = request(64).replace(number_field(10, 7), number_field(10, 8), 1)
        paid = request(1, continued=True).replace(number_field(10, 7), number_field(10, 8), 1)
        with self.assertRaises(AssertionError):
            fixture.verify_template(before, paid)

    def test_template_rejects_changed_named_tool_choice(self):
        with self.assertRaises(AssertionError):
            fixture.verify_template(request(64), request(1, continued=True, choice="Other"))

    def test_template_requires_named_tool_choice_even_when_both_omit_it(self):
        with self.assertRaises(AssertionError):
            fixture.verify_template(request(64, choice=None), request(1, continued=True, choice=None))

    def test_template_rejects_changed_parallel_mode(self):
        with self.assertRaises(AssertionError):
            fixture.verify_template(request(64), request(1, continued=True, parallel=0))

    def test_template_rejects_duplicate_output_limit(self):
        paid = request(1, continued=True).replace(
            text_field(8, sampling(1)), text_field(8, sampling(1) + number_field(2, 1)), 1)
        with self.assertRaises(AssertionError):
            fixture.verify_template(request(64), paid)


if __name__ == "__main__":
    unittest.main()
