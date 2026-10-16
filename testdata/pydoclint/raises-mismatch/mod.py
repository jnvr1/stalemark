def check(a: int) -> int:
    """Validate a number.

    Args:
        a (int): The number.

    Returns:
        int: The number.

    Raises:
        KeyError: Never actually raised.
    """
    if a < 0:
        raise ValueError("negative")
    return a
