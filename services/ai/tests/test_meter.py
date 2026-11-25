import pytest

from app.meter import PRICE, credits_for
from app.worker import HANDLERS

CENT_HALF = 5_000  # micro-USD in one credit


@pytest.mark.parametrize(
    ("cost", "kind", "credits"),
    [
        (0, "summarise", 2),  # the one-credit minimum, at 1.75 times, rounds up to 2
        (CENT_HALF, "generate_readme", 2),
        (CENT_HALF + 1, "generate_readme", 4),  # 2 base credits x 1.75 = 3.5, rounded up
        (4 * CENT_HALF, "readme_section", 7),
        (0, "generate_diagram", 3),  # 1 x 2.5 = 2.5, rounded up
        (2 * CENT_HALF, "generate_diagram", 5),
        (3 * CENT_HALF, "generate_diagram", 8),
    ],
)
def test_credits_for_applies_the_kind_price_and_rounds_up(cost: int, kind: str, credits: int) -> None:
    assert credits_for(cost, kind) == credits


def test_every_job_kind_has_a_price() -> None:
    assert set(PRICE) == set(HANDLERS)


def test_an_unknown_kind_is_refused_rather_than_charged_at_a_guess() -> None:
    with pytest.raises(KeyError):
        credits_for(1_000, "something_new")


@pytest.mark.parametrize(("kind", "hold"), [("summarise", 18), ("generate_readme", 27), ("readme_section", 4)])
def test_the_core_holds_are_the_earlier_holds_at_the_new_price(kind: str, hold: int) -> None:
    earlier = {"summarise": 10, "generate_readme": 15, "readme_section": 2}[kind]
    numerator, denominator = PRICE[kind]
    assert hold == -(-earlier * numerator // denominator)


def test_the_diagram_hold_is_the_earlier_hold_at_the_diagram_price() -> None:
    numerator, denominator = PRICE["generate_diagram"]
    assert -(-15 * numerator // denominator) == 38
