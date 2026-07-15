import React, { useState, useEffect, useRef } from 'react';
import { Form } from 'react-bootstrap';

function FilterableDropdown({
  options,
  value,
  onChange,
  placeholder = "Select an option...",
  disabled = false,
  required = false
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [filter, setFilter] = useState('');
  const [filteredOptions, setFilteredOptions] = useState(options);
  const dropdownRef = useRef(null);

  useEffect(() => {
    setFilteredOptions(
      options.filter(option =>
        option.label.toLowerCase().includes(filter.toLowerCase()) ||
        option.value.toLowerCase().includes(filter.toLowerCase())
      )
    );
  }, [filter, options]);

  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  const handleSelect = (option) => {
    onChange(option.value);
    setFilter(option.label);
    setIsOpen(false);
  };

  const handleInputChange = (e) => {
    setFilter(e.target.value);
    setIsOpen(true);
    if (value) {
      onChange(''); // Clear selection when user starts typing
    }
  };

  const handleInputFocus = () => {
    setIsOpen(true);
  };

  const selectedOption = options.find(opt => opt.value === value);
  const displayValue = selectedOption ? selectedOption.label : filter;

  return (
    <div ref={dropdownRef} className="filterable-dropdown">
      <Form.Control
        type="text"
        value={displayValue}
        onChange={handleInputChange}
        onFocus={handleInputFocus}
        placeholder={placeholder}
        disabled={disabled}
        required={required}
        autoComplete="off"
      />
      {isOpen && !disabled && (
        <div className="filterable-dropdown-menu">
          {filteredOptions.length === 0 ? (
            <div className="filterable-dropdown-no-results">
              No options found
            </div>
          ) : (
            filteredOptions.map((option) => (
              <div
                key={option.value}
                onClick={() => handleSelect(option)}
                className={`filterable-dropdown-item ${option.value === value ? 'selected' : ''}`}
              >
                {option.label}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}

export default FilterableDropdown;
