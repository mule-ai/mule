import main

def test_addition():
    assert main.add(5, 3) == 8,
    'Addition of 5 and 3 should be 8'
def test_subtraction():
    assert main.subtract(10, 4) == 6,
    'Subtraction of 10 and 4 should be 6'

if __name__ == '__main__':
    import unittest
    suite = unittest.TestLoader().loadTestsFromTestCase(main)
    runner = unittest.TextTestRunner()
    runner.run(suite)
