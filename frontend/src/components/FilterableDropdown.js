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
    <div ref={dropdownRef} style={{ position: 'relative' }}>
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
        <div 
          style={{
            position: 'absolute',
            top: '100%',
            left: 0,
            right: 0,
            zIndex: 1000,
            backgroundColor: 'white',
            border: '1px solid #ced4da',
            borderTop: 'none',
            borderRadius: '0 0 0.375rem 0.375rem',
            maxHeight: '200px',
            overflowY: 'auto',
            boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)'
          }}
        >
          {filteredOptions.length === 0 ? (
            <div style={{ padding: '8px 12px', color: '#6c757d' }}>
              No options found
            </div>
          ) : (
            filteredOptions.map((option) => (
              <div
                key={option.value}
                onClick={() => handleSelect(option)}
                style={{
                  padding: '8px 12px',
                  cursor: 'pointer',
                  backgroundColor: option.value === value ? '#e7f3ff' : 'white',
                  borderBottom: '1px solid #f8f9fa'
                }}
                onMouseEnter={(e) => {
                  if (option.value !== value) {
                    e.target.style.backgroundColor = '#f8f9fa';
                  }
                }}
                onMouseLeave={(e) => {
                  e.target.style.backgroundColor = option.value === value ? '#e7f3ff' : 'white';
                }}
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
